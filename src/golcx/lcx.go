package main

import (
	"flag"
	"log"
	"net"
	"tunnel"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)
}
func main() {
	var listenServer, listenClient, backendAddr, localAddr, mode string
	flag.StringVar(&listenServer, "listen-server", ":8888", "host:port server listen on")
	flag.StringVar(&listenClient, "listen-client", ":8889", "host:port client listen on")
	flag.StringVar(&backendAddr, "backend-addr", "127.0.0.1:8888", "host:port of the backend server")
	flag.StringVar(&localAddr, "local-addr", "127.0.0.1:80", "host:port of the local server")
	flag.StringVar(&mode, "mode", "client", "client or server mode")
	flag.Parse()

	if mode == "client" {
		startClient(backendAddr, localAddr)
	} else {
		startServer(listenServer, listenClient)
	}

}

func startClient(backendAddr, localAddr string) {
	//connect remote addr
	addr, err := net.ResolveTCPAddr("tcp", backendAddr)
	if err != nil {
		log.Fatal(err)
	}
	conn, err2 := net.DialTCP("tcp", nil, addr)
	if err2 != nil {
		panic("remote server is not ready or listening")
	}
	bundle := tunnel.NewBundle(conn)

	//connect local addr
	addr, err = net.ResolveTCPAddr("tcp", localAddr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		var header [4]byte
		_, err := bundle.Read(header[:])
		if err != nil {
			log.Fatal(err)
		}
		id := int(header[0])<<8 | int(header[1])
		length := int(header[2])<<8 | int(header[3])
		tunnel := bundle.GetId(id)
		if tunnel == nil {
			conn2, err2 := net.DialTCP("tcp", nil, addr)
			if err2 != nil {
				log.Fatal(err2)
				continue
			}
			tunnel = bundle.Alloc(conn2)
		}
		if length == 0 {
			bundle.Free(id)
			continue
		}
		log.Println("bundle revieve a buf len=", length, ",id=", id)
		buf := make([]byte, length)
		bundle.Read(buf[:])
		tunnel.Send <- buf
	}

}

func startServer(listenServer, listenClient string) {
	//listen server addr and block until a request come
	addr, err := net.ResolveTCPAddr("tcp", listenServer)
	if err != nil {
		log.Fatal(err)
	}
	listen, err2 := net.ListenTCP("tcp", addr)
	if err2 != nil {
		log.Fatal(err2)
	}
	conn, err3 := listen.AcceptTCP()
	if err3 != nil {
		log.Fatal(err2)
	}
	bundle := tunnel.NewBundle(conn)

	go func() {
		for {
			var header [4]byte
			_, err := bundle.Read(header[:])
			if err != nil {
				log.Fatal(err)
			}
			id := int(header[0])<<8 | int(header[1])
			length := int(header[2])<<8 | int(header[3])
			tunnel := bundle.GetId(id)
			if tunnel == nil && length != 0 {
				conn2, err2 := net.DialTCP("tcp", nil, addr)
				if err2 != nil {
					log.Fatal(err2)
					continue
				}
				tunnel = bundle.Alloc(conn2)
			}
			if length == 0 {
				bundle.Free(id)
				continue
			}
			log.Println("bundle revieve a buf len=", length, ",id=", id)
			buf := make([]byte, length)
			bundle.Read(buf[:])
			tunnel.Send <- buf
		}
	}()

	//now listen client addr and recieve request now
	addr, err = net.ResolveTCPAddr("tcp", listenClient)
	if err != nil {
		log.Fatal(err)
	}
	listen, err2 = net.ListenTCP("tcp", addr)
	if err2 != nil {
		log.Fatal(err2)
	}
	for {
		conn2, err := listen.AcceptTCP()
		if err != nil {
			log.Fatal(err)
		}
		bundle.Alloc(conn2)
	}
}
