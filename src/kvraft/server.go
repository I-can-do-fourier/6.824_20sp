package kvraft

//完成了lab3ab,修复了raft提速后产生的一系列bug。

import (
	"bytes"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"../labgob"
	"../labrpc"
	"../raft"
)

const Debug = 0

// var CliMap map[uint64]ClientInfo = make(map[uint64]ClientInfo)
// var muCli sync.Mutex

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.

	Id int //client id

	Num int //the number of the request

	Type  int //0:get 1:put 2:append -1:not such key
	Key   string
	Value string //if string=="",then op==get

}

type ClientInfo struct {
	Num     int  //the lastest request num
	Index   int  //the promised index for that request
	Term    int  //the promised term for that request
	Applied bool //yes or no applied

}

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug > 0 {
		log.Printf(format, a...)
	}
	return
}

type KVServer struct {
	mu sync.Mutex
	me int
	rf *raft.Raft

	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.

	applier    chan Op
	consumers  []chan Op
	consumers2 chan chan Op

	mp map[string]string //database

	clientNum map[uint64]int //每个client请求的最新值

	ClientCh map[int][]chan Op //applier和client-server rpc的channel。一个index,可能有多个client在等。需优化(踢掉之前等待的client)
	mucli    sync.Mutex

	//latestApplied int //最新applied的Index

	CliMap map[int]int //存着client最新的request Num

	lastApplied int        //最后一个被applied的index
	mumap       sync.Mutex //为操作map,cliMap和index设置的锁

	snapshotIndex int
	snapshotTerm  int

	persister *raft.Persister
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.

	// if kv.rf.tp!=2{

	// 	reply.Err="ErrWrongLeader"
	// 	return
	// }

	//kv.mu.Lock()

	//kv.checkSnapBytes()不能将这个函数放在这里或者PutAppend里，不然可能会被连续调用

	command := Op{args.ClientId, args.Num, 0, args.Key, ""}

	// num, ok := kv.clientNum[args.clientId]

	// if ok && num >= args.Num {

	// 	reply.Err = "ErrWrongLeader"
	// 	DPrintf("kvserver %v:already submit the request:%v\n", kv.me, command)
	// 	kv.mu.Unlock()
	// 	return

	// }

	//index, term, isLeader := kv.rf.Start(command)
	//oldNum:=

	if kv.killed() {

		return
	}

	DPrintf("kvserver %v:check 1\n", kv.me)

	index, _, isLeader := kv.rf.Start(command)

	if !isLeader {

		reply.Err = "ErrWrongLeader"
		DPrintf("kvserver %v:get fail,not a leader,command:%v\n", kv.me, command)
		//kv.mu.Unlock()
		return

	} else {

		//kv.clientNum[args.clientId] = args.Num

		ch := make(chan Op)
		//kv.consumers = append(kv.consumers, ch)

		//kv.consumers2 <- ch
		kv.mucli.Lock()

		//index = index + kv.snapshotIndex
		DPrintf("kvserver %v:into the unknown1 :%v\n", kv.me, command)

		kv.mumap.Lock()
		info, okk := kv.CliMap[args.ClientId]
		DPrintf("kvserver %v:into the unknown2 :%v\n", kv.me, command)
		kv.mumap.Unlock()
		DPrintf("kvserver %v:into the unknown3 :%v\n", kv.me, command)

		/**

		必要的检查，因为这个是有server可能已经apply这个command

		**/
		if okk && info >= args.Num {

			reply.Err = "OK"

			kv.mumap.Lock()
			reply.Value = kv.mp[args.Key]
			kv.mumap.Unlock()

			DPrintf("kvserver %v:command has been used :%v\n", kv.me, command)

			kv.mucli.Unlock()
			return
			//reply.Value = cm.Value

		}

		DPrintf("kvserver %v:create ch :%v\n", kv.me, command)

		item, ok := kv.ClientCh[index]

		if ok {

			kv.ClientCh[index] = append(item, ch)

		} else {

			kv.ClientCh[index] = []chan Op{ch}

		}

		//kv.ClientCh[index] = ch
		kv.mucli.Unlock()

		DPrintf("kvserver %v:prepare get,rank:%v,index:%v,command %v\n", kv.me, len(kv.consumers)-1, index, command)

		//kv.mu.Unlock()
		DPrintf("kvserver %v:check 2\n", kv.me)
		cm := <-ch

		if kv.killed() {

			return
		}

		// if cm.Type == -3 {

		// 	kv.mumap.Lock()

		// 	info, okk := kv.CliMap[args.ClientId]

		// 	kv.mumap.Unlock()

		// 	if okk && info >= args.Num {

		// 		reply.Err = "OK"

		// 	} else {

		// 		reply.Err = "discarded"
		// 	}

		// }

		if cm.Type == -1 {

			reply.Err = "ErrNoKey"
			//kv.mu.Unlock()
			return

		} else if cm.Type != command.Type || cm.Id != args.ClientId || cm.Num != args.Num {

			reply.Err = "command fail"
			//kv.mu.Unlock()
			return

		}

		reply.Err = "OK"
		reply.Value = cm.Value

	}

}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.

	//kv.checkSnapBytes()
	var command Op
	if args.Op == "Put" {

		command = Op{args.ClientId, args.Num, 1, args.Key, args.Value}

	} else {

		command = Op{args.ClientId, args.Num, 2, args.Key, args.Value}

	}

	//kv.mu.Lock()

	if kv.killed() {

		return
	}
	DPrintf("kvserver %v:check 3\n", kv.me)
	index, _, isLeader := kv.rf.Start(command)

	if !isLeader {

		reply.Err = "ErrWrongLeader"
		DPrintf("kvserver %v:PutAppend fail,not a leader,command:%v\n", kv.me, command)
		//kv.mu.Unlock()
		return

	} else {

		//kv.clientNum[args.ClientId] = args.Num

		ch := make(chan Op)
		//kv.consumers = append(kv.consumers, ch)

		kv.mucli.Lock()

		kv.mumap.Lock()
		info, okk := kv.CliMap[args.ClientId]
		kv.mumap.Unlock()

		/**

		必要的检查，因为这个是有server可能已经apply这个command

		**/

		if okk && info >= args.Num {

			reply.Err = "OK"
			DPrintf("kvserver %v:command has been used append:%v\n", kv.me, command)
			kv.mucli.Unlock()
			return
			//reply.Value = cm.Value

		}

		DPrintf("kvserver %v:create ch :%v\n", kv.me, command)

		//index = index + kv.snapshotIndex

		item, ok := kv.ClientCh[index]

		if ok {

			kv.ClientCh[index] = append(item, ch)

		} else {

			kv.ClientCh[index] = []chan Op{ch}

		}

		//kv.ClientCh[index] = ch
		kv.mucli.Unlock()
		//kv.consumers2 <- ch

		DPrintf("kvserver %v:prepare putAppend,rank:%v,index:%v,command %v\n", kv.me, len(kv.consumers)-1, index, command)
		//kv.mu.Unlock()

		DPrintf("kvserver %v:check 4,index:%v\n", kv.me, index)
		cm := <-ch
		DPrintf("kvserver %v:append receive,%v\n", kv.me, cm)
		if kv.killed() {

			return
		}

		if cm.Type == -3 {

			kv.mumap.Lock()

			info, okk := kv.CliMap[args.ClientId]

			kv.mumap.Unlock()

			if okk && info >= args.Num {

				reply.Err = "OK"

			} else {

				reply.Err = "discarded"
			}

			return

		}

		if cm.Type != command.Type || cm.Id != args.ClientId || cm.Num != args.Num {

			reply.Err = "command fail"
			//kv.mu.Unlock()
			return

		}

		reply.Err = "OK"
		//reply.Value = cm.Value

	}
}

//
// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
//
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

//
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate
	kv.persister = persister

	// You may need initialization code here.

	kv.mp = make(map[string]string)
	kv.consumers = make([]chan Op, 0)
	kv.consumers2 = make(chan chan Op, 100)

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)
	kv.rf.Noop = true

	kv.clientNum = make(map[uint64]int)

	kv.CliMap = make(map[int]int)
	kv.ClientCh = make(map[int][]chan Op)

	// You may need initialization code here.

	ss := make(chan raft.SnapShot, 1)

	kv.rf.Ss = ss

	kv.snapshotIndex = 0
	kv.lastApplied = 0

	snapsync := make(chan bool)
	kv.rf.SnapSync = snapsync

	snapsync2 := make(chan bool)
	kv.rf.SnapSync2 = snapsync2

	kv.readSnap(persister.ReadSnapshot())

	//kv.lastApplied = kv.snapshotIndex

	//kv.persister=persister

	go func() {

		kv.Applier()

	}()

	go func() {

		for {

			if kv.maxraftstate != -1 && persister.RaftStateSize() > kv.maxraftstate/3*2 {

				DPrintf("kvserver %v:start discard\n", kv.me)

				start := time.Now()

				kv.mumap.Lock()

				DPrintf("kvserver %v:check 20\n", kv.me)

				//kv.snapshotIndex = kv.lastApplied

				mmap := make(map[string]string)
				climap := make(map[int]int)

				for k, v := range kv.mp {

					mmap[k] = v

				}

				for k, v := range kv.CliMap {

					climap[k] = v
				}

				//ss需要设置为buffered channel,如果里面有
				// if len(ss) > 0 {

				// 	time.Sleep(400 * time.Millisecond)
				// 	continue
				// }

				if kv.killed() {

					return
				}

				DPrintf("kvserver %v:check 21\n", kv.me)

				ss <- raft.SnapShot{Mp: mmap, CliMap: climap, LastIndex: kv.lastApplied}

				if kv.killed() {

					return
				}

				kv.mumap.Unlock()

				if kv.killed() {

					return
				}
				DPrintf("kvserver %v:check 22\n", kv.me)
				<-snapsync

				//kv.mumap.Unlock() 放在这会🤣死锁

				DPrintf("kvserver %v:server snap:%v\n", kv.me, time.Since(start))
			}

			time.Sleep(40 * time.Millisecond)

		}

	}()

	DPrintf("kvserver %v:init\n", kv.me)

	return kv
}

func (kv *KVServer) Applier() {

	for {

		if kv.killed() {

			return
		}

		DPrintf("kvserver %v:check 5\n", kv.me)
		msg := <-kv.applyCh
		//var cm Op

		if !msg.CommandValid {

			start := time.Now()

			DPrintf("kvserver %v:install snapshot%v\n", kv.me, msg)

			sn := msg.Command.([]byte)

			r := bytes.NewBuffer(sn)
			d := labgob.NewDecoder(r)
			var snapshot raft.SnapShot

			d.Decode(&snapshot)

			//kv.snapshotIndex = snapshot.LastIndex
			//kv.snapshotTerm = snapshot.LastTerm
			kv.mp = snapshot.Mp
			kv.CliMap = snapshot.CliMap

			if snapshot.LastIndex > kv.lastApplied {

				kv.lastApplied = snapshot.LastIndex

			}
			DPrintf("kvserver %v:check 11\n", kv.me)
			kv.mucli.Lock()

			for k, v := range kv.ClientCh {

				if k <= snapshot.LastIndex {

					for _, c := range v {

						//if kv.CliMap
						DPrintf("kvserver %v:fuck an entry%v,%v\n", kv.me, msg, k)
						c <- Op{Id: -2, Type: -3}

					}
				}

			}

			DPrintf("kvserver %v:check 6\n", kv.me)
			kv.mucli.Unlock()

			kv.rf.SnapSync2 <- true

			DPrintf("kvserver %v:check 12\n", kv.me)

			DPrintf("kvserver %v:server get snap:%v\n", kv.me, time.Since(start))

			continue

		}

		var cm Op

		switch t := msg.Command.(type) {

		case string:

			cm = Op{Id: -1, Type: -2}
			DPrintf("kvserver %v:get a no-op%v\n", kv.me, t)
		case Op:
			cm = msg.Command.(Op)
			//continue

		}

		DPrintf("kvserver %v:try to apply a command%v\n", kv.me, msg)

		// if cm.Id == -1 {

		// 	DPrintf("kvserver %v:get a no-op%v\n", kv.me, msg)
		// }

		//muCli.Lock()

		DPrintf("kvserver %v:check 7\n", kv.me)

		kv.mumap.Lock()

		info, okk := kv.CliMap[cm.Id]

		//muCli.Unlock()

		kv.lastApplied = msg.CommandIndex //+ kv.snapshotIndex

		if cm.Type == 0 {

			s, ok := kv.mp[cm.Key]
			if !ok {

				cm.Type = -1
				//ch <- cm

			} else {

				cm.Value = s
				//ch <- cm

			}

			kv.CliMap[cm.Id] = cm.Num

		} else if cm.Type == 1 {

			if okk && info >= cm.Num {

				DPrintf("kvserver %v:already executed%v\n", kv.me, msg)

			} else {

				kv.mp[cm.Key] = cm.Value
				kv.CliMap[cm.Id] = cm.Num
			}

			//ch <- cm

		} else if cm.Type == 2 {

			if okk && info >= cm.Num {

				DPrintf("kvserver %v:already executed%v\n", kv.me, msg)

			} else {

				s, ok := kv.mp[cm.Key]

				if !ok {

					kv.mp[cm.Key] = cm.Value
				} else {

					kv.mp[cm.Key] = s + cm.Value

				}

				kv.CliMap[cm.Id] = cm.Num

			}

			//ch <- cm

		}

		DPrintf("kvserver %v:print map:%v\n", kv.me, kv.mp)

		DPrintf("kvserver %v:check 8\n", kv.me)
		kv.mumap.Unlock()

		if kv.killed() {

			return
		}

		DPrintf("kvserver %v:check 9,index:%v\n", kv.me, msg.CommandIndex)
		kv.mucli.Lock()

		chs, ok := kv.ClientCh[msg.CommandIndex]

		if ok {

			for _, ch := range chs {

				if kv.killed() {

					return
				}

				DPrintf("kvserver %v:give to client1,%v,index:%v\n", kv.me, cm, msg.CommandIndex)
				ch <- cm
				DPrintf("kvserver %v:give to client2,%v,index:%v\n", kv.me, cm, msg.CommandIndex)
			}

			//ch <- cm
			delete(kv.ClientCh, msg.CommandIndex)

		}

		DPrintf("kvserver %v:check 10\n", kv.me)
		kv.mucli.Unlock()

		// if len(kv.consumers2) > 0 {

		// 	// kv.mu.Lock()
		// 	// ch := kv.consumers[0]
		// 	// kv.consumers = kv.consumers[1:]
		// 	// kv.mu.Unlock()

		// 	ch := <-kv.consumers2

		// 	ch <- cm

		// }

	}

}

func (kv *KVServer) readSnap(data []byte) {

	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var snapshot raft.SnapShot

	d.Decode(&snapshot)

	//kv.snapshotIndex = snapshot.LastIndex
	//kv.snapshotTerm = snapshot.LastTerm

	if snapshot.Mp != nil {

		kv.mp = snapshot.Mp
	}

	if snapshot.CliMap != nil {

		kv.CliMap = snapshot.CliMap

	}

	kv.lastApplied = snapshot.LastIndex

}

func (kv *KVServer) checkSnapBytes() {

	if kv.maxraftstate != -1 && kv.rf.GetPersister().RaftStateSize() > kv.maxraftstate/3*2 {

		DPrintf("kvserver %v:start discard\n", kv.me)

		start := time.Now()

		kv.mumap.Lock()

		//kv.snapshotIndex = kv.lastApplied
		kv.rf.Ss <- raft.SnapShot{Mp: kv.mp, CliMap: kv.CliMap, LastIndex: kv.lastApplied}

		<-kv.rf.SnapSync

		kv.mumap.Unlock()

		DPrintf("kvserver %v:server snap:%v\n", kv.me, time.Since(start))
	}

}

// func (kv *KVServer) saveStates() {

// 	kv.mumap.Lock()
// 	defer kv.mumap.Unlock()

// 	ss := raft.SnapShot{Mp: kv.mp, CliMap: kv.CliMap, LastIndex: kv.lastApplied}

// 	w1 := new(bytes.Buffer)
// 	e1 := labgob.NewEncoder(w1)
// 	e1.Encode(ss)

// 	snapdata := w1.Bytes()
// 	// DPrintf("id:%v,term:%v,tp:%v,check 7", rf.me, rf.currentTerm, rf.tp)
// 	// rf.persister.SaveStateAndSnapshot(statedata, snapdata)

// }
