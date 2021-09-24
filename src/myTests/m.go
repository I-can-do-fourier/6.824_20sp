//test rcp pass channel
//master

package myTests

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
)

type Master struct {
	// Your definitions here.

	Send    chan int
	Receive chan int
}

func M() {

	m := Master{make(chan int, 5), make(chan int, 5)}

	go func() {

		m.server()

	}()

	for i := 0; i < 10; i++ {

		fmt.Println("sending: ", i)
		m.Send <- i

	}

	for i := 0; i < 10; i++ {

		fmt.Println("receive: ", <-m.Receive)

	}

}

func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

func (m *Master) Example(args *ExampleArgs,
	reply *ExampleReply) error {

	reply.Send = m.Send
	reply.Receive = m.Receive
	return nil
}
