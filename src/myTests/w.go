//test rcp pass channel
//worker

package myTests

import (
	"fmt"
	"log"
	"net/rpc"
)

func Worker() {

	//arg := &ExampleArgs{1}
	CallExample()

}

func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{1}

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	reply.Receive <- (<-reply.Send)
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
