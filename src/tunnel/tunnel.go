package tunnel

import (
	"container/list"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const socksServer = "127.0.0.1:1080"
const bindAddr = ":2011"
const bufferSize = 4096
const maxConn = 0x10000
const xor = 0x64

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}

type Bundle struct {
	t [maxConn]tunnel
	*list.List
	*xsocket
	sync.Mutex
}

type tunnel struct {
	id int
	*list.Element
	Send  chan []byte
	reply io.Writer
	conn  net.Conn
}

type xsocket struct {
	net.Conn
	*sync.Mutex
}

func NewBundle(c net.Conn) *Bundle {
	b := new(Bundle)
	b.List = list.New()
	for i := 0; i < maxConn; i++ {
		t := &b.t[i]
		t.id = i
		t.Element = b.List.PushBack(t)
	}
	b.xsocket = &xsocket{c, new(sync.Mutex)}
	return b
}

func (b *Bundle) Alloc(c net.Conn) *tunnel {
	f := b.List.Front()
	if f == nil {
		return nil
	}
	t := b.List.Remove(f).(*tunnel)
	t.Element = nil
	log.Println("alloc a new conn,id:", t.id)
	t.open(b)
	t.conn = c
	return t
}

func (b *Bundle) Free(id int) {
	log.Println("free a tunnel,id:", id)
	b.Lock()
	defer b.Unlock()
	t := &b.t[id]
	if t.Element == nil {
		t.Element = b.PushBack(t)
		t.close()
	}
}

func (b *Bundle) GetId(id int) (t *tunnel) {
	b.Lock()
	defer b.Unlock()
	t = &b.t[id]
	//t 为未使用的情况下
	if t.Element != nil {
		return nil
	}
	return t
}

func (s xsocket) Read(data []byte) (n int, err error) {
	n, err = io.ReadFull(s.Conn, data)
	//简单解密
	if n > 0 {
		for i := 0; i < n; i++ {
			data[i] = data[i] ^ xor
		}
	}

	return
}

func (s xsocket) Write(data []byte) (n int, err error) {
	s.Lock()
	defer s.Unlock()
	id := int(data[0])<<8 | int(data[1])
	length := int(data[2])<<8 | int(data[3])
	log.Println("xsocket write from ", id, " len=", length)
	log.Println("xsocket Send", len(data))
	for i := 0; i < len(data); i++ {
		data[i] = data[i] ^ xor
	}

	x := 0
	all := len(data)

	for all > 0 {
		n, err = s.Conn.Write(data)
		if err != nil {
			n += x
			return
		}
		if all != n {
			log.Println("Write only", n, all)
		}
		all -= n
		x += n
		data = data[n:]
	}
	return
	// return s.Conn.Write(data)
}

func (t *tunnel) open(b *Bundle) {
	t.Send = make(chan []byte)
	t.reply = b.xsocket
	go t.process(b)
}

func (t *tunnel) close() {
	close(t.Send)
}

func (t *tunnel) sendClose() {
	log.Println("closing a remote tunnel id:", t.id)
	buf := [4]byte{
		byte(t.id >> 8),
		byte(t.id & 0xff),
		0,
		0,
	}
	t.reply.Write(buf[:])
}

func (t *tunnel) sendBack(buf []byte) {
	buf[0] = byte(t.id >> 8)
	buf[1] = byte(t.id & 0xff)
	length := len(buf) - 4
	buf[2] = byte(length >> 8)
	buf[3] = byte(length & 0xff)
	t.reply.Write(buf[:])
}

func (t *tunnel) process(b *Bundle) {
	go t.processBack()
	send := t.Send
	for {
		buf, ok := <-send
		if !ok {
			if t.conn != nil {
				t.conn.Close()
			}
			return
		}
		if t.conn != nil {
			if n, err := t.conn.Write(buf); err != nil {
				log.Println(t.id, "err", err)
				b.Free(t.id)
			} else {
				log.Println(t.id, ":tunnel Write", n, len(buf))
			}
		}
	}
}

func (t *tunnel) processBack() {
	var buf [bufferSize]byte
	for {
		t.conn.SetReadDeadline(time.Now().Add(time.Minute * 20))
		n, err := t.conn.Read(buf[4:])
		log.Printf("tunnel : %d revieve a buf len=%d\n", t.id, n)
		if n > 0 {
			t.sendBack(buf[:n+4])
		}
		e, ok := err.(net.Error)
		if !ok && err != nil {
			log.Println(t.id, n, err, e, ok)
			return
		}
	}
}
