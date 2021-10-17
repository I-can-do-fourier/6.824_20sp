package kvraft

import (
	"crypto/rand"
	"math/big"
	"sync/atomic"
	"time"

	"../labrpc"
)

var id int64 //the id of the client

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	number int //the number of the action

	leader int //the id of the pre leader
	me     int

	//mu
}

//client 要求的command
type ClientRequestArg struct {
	Type int //0:get 1:put 2:append

	Key   string
	Value string
}

type ClientRequestReply struct {
	Error     string
	Type      int    //0:get 1:put 2:append
	GetResult string //get到的东西，如果没有这个key，return empty string

}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// You'll have to add code here.

	//ck.me = atomic.LoadUint64(id)
	ck.me = int(atomic.AddInt64(&id, 1))
	ck.me--

	return ck
}

//
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) Get(key string) string {

	// You will have to modify this function.

	ck.number++
	num := ck.number

	leader := ck.leader

	arg := &GetArgs{key, num, ck.me}
	reply := &GetReply{}

	for {

		DPrintf("client %v num:%v,want to get :%v\n", ck.me, num, arg)

		ok := ck.servers[leader].Call("KVServer.Get", arg, reply)
		if !ok || reply.Err == "ErrWrongLeader" || reply.Err == "command fail" {

			DPrintf("client %v num:%v,get error:%v,pre_leader:%v,arg:%v\n", ck.me, num, reply.Err, leader, arg)
			//leader++
			leader = (leader + 1) % len(ck.servers)

		} else if ok && reply.Err == "ErrNoKey" {
			DPrintf("client %v num:%v,get error:%v,pre_leader:%v,arg:%v\n", ck.me, num, reply.Err, leader, arg)
			ck.leader = leader

			return ""

		} else if ok && reply.Err == "OK" {
			DPrintf("client %v num:%v,get the value k:%v,v:%v\n", ck.me, num, key, reply.Value)
			ck.leader = leader
			return reply.Value //如果getResult是空的,代表没有这个key

		}

		time.Sleep(30 * time.Millisecond)

	}
	//return ""
}

//
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.

	ck.number++
	num := ck.number

	leader := ck.leader

	// var tp int
	// if op == "Put" {

	// 	tp = 1
	// } else if op == "Append" {

	// 	tp = 2
	// }

	//arg := &ClientRequestArg{Type: tp, Key: key, Value: value}
	arg := &PutAppendArgs{key, value, op, num, ck.me}
	reply := &PutAppendReply{}

	for {

		DPrintf("client %v num:%v,want to append :%v\n", ck.me, num, arg)

		ok := ck.servers[leader].Call("KVServer.PutAppend", arg, reply)
		if !ok || reply.Err == "ErrWrongLeader" {

			DPrintf("client %v num:%v,%v error:%v,pre_leader:%v,arg:%v\n", ck.me, num, op, reply.Err, leader, arg)
			leader = (leader + 1) % len(ck.servers)

		} else if reply.Err == "command fail" {

			DPrintf("client %v num:%v,%v error:%v,pre_leader:%v,arg:%v\n", ck.me, num, op, reply.Err, leader, arg)
			leader = (leader + 1) % len(ck.servers)
			//ck.number++
			//num = ck.number

		} else if reply.Err == "Already" {

			DPrintf("client %v num:%v,already summit,%v the value k:%v,v:%v\n", ck.me, num, op, key, arg.Value)
			ck.leader = leader

			return

		} else if ok && reply.Err == "OK" {
			DPrintf("client %v num:%v,%v the value k:%v,v:%v\n", ck.me, num, op, key, arg.Value)
			ck.leader = leader

			return

		}

		time.Sleep(30 * time.Millisecond)

	}

}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
