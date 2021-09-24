package raft

//使用channel+timer
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"math/rand"

	"../haha"
	"../labgob"
	"../labrpc"
)

var m haha.Ha

// import "bytes"
// import "../labgob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	//type

	tp int //type 0:follower  1:candidate  2:leader

	//persistent state
	currentTerm int
	votedFor    int
	log         []Log

	//volatile state

	commitIndex int
	lastApplied int

	//volatile on leaders

	nextIndex  []int
	matchIndex []int

	//for candidate

	voteCount int

	timePassed int //time passed since last request from a leader. milliseconds
	timeout    int

	commitCount []int //统计commitindex 之后的replicate的log的数量

	appendFlag bool //当接收到append success的信号时，通知updateCommit去更新commitIndex
	appC       *sync.Cond

	applych chan ApplyMsg

	startAppend bool //当leader收到启动后的第一个client来的请求时，置位true
}

//self definition for log[]
type Log struct {
	Command interface{}
	Term    int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).

	rf.mu.Lock()
	term = rf.currentTerm
	if rf.tp == 2 {

		isleader = true

	} else {
		isleader = false
	}

	rf.mu.Unlock()

	//不能写成return rf.currentTerm,rf.....,因为不方便加锁。似乎
	//但是似乎可以在return之后释放锁
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)

	// //persistent state
	// currentTerm int
	// votedFor    int
	// log         []Log

	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)

}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }

	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm int
	var voteFor int
	var l []Log

	if d.Decode(&currentTerm) != nil || d.Decode(&voteFor) != nil || d.Decode(&l) != nil {

		log.Fatalf("decode fail")

	} else {

		rf.currentTerm = currentTerm
		rf.votedFor = voteFor
		rf.log = l

	}

	// //persistent state
	// currentTerm int
	// votedFor    int
	// log         []Log

}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).

	Term        int
	VoteGranted bool
}

type RequestAppendArgs struct {
	// Your data here (2A, 2B).
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int

	Entries []Log

	LeaderCommit int
}

type RequestAppendReply struct {
	// Your data here (2A).

	Term    int
	Success bool

	//for fast nextIndex match

	Miss         bool //如果leader发送的preLogIndex超出了我log的范围，则视为miss、
	TermConflict int  //如果发生了conflict,需要告诉leader发生conflict位置的term
	TermStart    int  //如果发生了conflict,需要告诉leader发生conflict位置的term的起始index
	Len          int  //我的log的长度
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).

	rf.mu.Lock()

	log.Printf("id:%v,term:%v,tp:%v,receive vote request from %v(term:%v)\n",
		rf.me, rf.currentTerm, rf.tp, args.CandidateId, args.Term)

	reply.Term = rf.currentTerm

	if args.Term < rf.currentTerm {

		reply.VoteGranted = false

		rf.mu.Unlock()
		return
	}

	if args.Term > rf.currentTerm {

		rf.currentTerm = args.Term
		//rf.persist()
		rf.tp = 0
		rf.votedFor = -1

		rf.persist()

	}

	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {

		if args.LastLogTerm > rf.log[len(rf.log)-1].Term ||
			(args.LastLogTerm == rf.log[len(rf.log)-1].Term && args.LastLogIndex >= len(rf.log)-1) {

			reply.VoteGranted = true
			rf.votedFor = args.CandidateId
			rf.persist()
			rf.resetTime()

		} else {

			reply.VoteGranted = false
		}

	} else {
		reply.VoteGranted = false
	}

	rf.mu.Unlock()

	return

}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	rf.mu.Lock()
	if rf.tp != 2 {

		log.Printf("id:%v,term:%v,tp:%v,reject the client\n",
			rf.me, rf.currentTerm, rf.tp)
		rf.mu.Unlock()
		return index, term, false
	}
	term = rf.currentTerm

	rf.log = append(rf.log, Log{command, rf.currentTerm})

	rf.persist()

	index = len(rf.log) - 1
	rf.matchIndex[rf.me] = index

	rf.startAppend = true

	log.Printf("id:%v,term:%v,tp:%v,receive a log from client,%v\n",
		rf.me, rf.currentTerm, rf.tp, command)

	// for i := 0; i < len(rf.peers); i++ {

	// 	if i != rf.me {

	// 		go func(server int) {

	// 			rf.Append2(server)

	// 		}(i)

	// 	}

	// }

	rf.mu.Unlock()
	return index, term, isLeader
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).

	rf.applych = applyCh

	// rf.currentTerm = 0
	// rf.votedFor = -1
	// rf.log = []Log{}
	// rf.log = append(rf.log, Log{Term: 0})

	rf.readPersist(rf.persister.raftstate)
	if rf.currentTerm == 0 {
		rf.votedFor = -1
	}

	if len(rf.log) == 0 {

		rf.log = append(rf.log, Log{Term: 0})

	}

	rf.persist()

	log.Printf("id:%v,term:%v,votedFor:%v,log:%v", rf.me, rf.currentTerm, rf.votedFor, rf.log)

	rf.commitIndex = 0
	rf.lastApplied = 0

	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	for i := 0; i < len(rf.peers); i++ {

		rf.nextIndex[i] = len(rf.log)
		rf.matchIndex[i] = 0 //应该不需要

	}

	rf.resetTime()

	rf.commitCount = make([]int, 0)

	rf.appC = sync.NewCond(&rf.mu)

	// initialize from state persisted before a crash
	//rf.readPersist(persister.ReadRaftState())

	go func() {

		rf.oscillator()
	}()

	return rf
}

//self functions

//update the term
func (rf *Raft) termUpdate(newTerm int, tp int, voteFor int) {

	rf.currentTerm = newTerm
	//rf.persist()
	rf.tp = tp
	rf.votedFor = voteFor

	rf.persist()

}

//control the time-out
func (rf *Raft) oscillator() {

	//rf.timeout = 300 + rand.Intn(400)

	inc := 10 //for sleep

	for {

		if rf.killed() {

			return
		}

		rf.mu.Lock()
		if rf.tp == 2 {
			rf.mu.Unlock()
			return

		}

		// log.Printf("id:%v,term:%v,tp:%v,strike\n",
		// 	rf.me, rf.currentTerm, rf.tp)

		if rf.timePassed >= rf.timeout {

			rf.resetTime()
			go func() {

				rf.startElection()

			}()

		}

		rf.mu.Unlock()
		time.Sleep(time.Duration(inc) * time.Millisecond)

		rf.mu.Lock()
		rf.timePassed = rf.timePassed + inc
		rf.mu.Unlock()

	}

}

//need modification
func (rf *Raft) sendAppendEntries(server int, args *RequestAppendArgs, reply *RequestAppendReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) AppendEntries(args *RequestAppendArgs, reply *RequestAppendReply) {

	rf.mu.Lock()

	reply.Term = rf.currentTerm

	if args.Term < rf.currentTerm {

		reply.Success = false
		rf.mu.Unlock()

		return

	} else if args.Term > rf.currentTerm {

		rf.termUpdate(args.Term, 0, -1)
		//rf.resetTime()

	} else if rf.tp == 1 && args.Term == rf.currentTerm {

		rf.tp = 0

		//rf.resetTime()

	} else { //

		//rf.resetTime()
	}

	if len(args.Entries) == 0 {

		//rf.mu.Unlock()
		//rf.resetTime()

		reply.Success = true

		log.Printf("id:%v,term:%v,tp:%v,receive heartbeat %v(term:%v),commitIndex:%v,log:%v\n",
			rf.me, rf.currentTerm, rf.tp, args.LeaderId, args.Term, rf.commitIndex, rf.log)

		//return

	}

	if args.PrevLogIndex < len(rf.log) && args.PrevLogTerm == rf.log[args.PrevLogIndex].Term {

		reply.Success = true
		//delete(rf.log,)

		for index, item := range args.Entries {

			if args.PrevLogIndex+1+index > len(rf.log)-1 ||
				rf.log[args.PrevLogIndex+1+index].Term != item.Term {

				temp := rf.log[0 : args.PrevLogIndex+1+index]
				rf.log = append(temp, args.Entries[index:]...)
				rf.persist()

				break
			} else {

				rf.log[args.PrevLogIndex+1+index] = args.Entries[index]
				rf.persist()

			}

		}

		if len(args.Entries) != 0 {

			log.Printf("id:%v,term:%v,tp:%v,appended from %v(term:%v),append result:%v\n",
				rf.me, rf.currentTerm, rf.tp, args.LeaderId, args.Term, rf.log)
			//rf.log = append(rf.log[0:args.PrevLogIndex+1], args.Entries...)

		}

	} else {

		reply.Success = false

		//fast roll back

		if args.PrevLogIndex >= len(rf.log) {

			reply.Miss = true
			reply.Len = len(rf.log)
			log.Printf("id:%v,term:%v,tp:%v,appended from %v(term:%v),miss,my length is:%v\n",
				rf.me, rf.currentTerm, rf.tp, args.LeaderId, args.Term, len(rf.log))

		} else {

			reply.TermConflict = rf.log[args.PrevLogIndex].Term
			reply.TermStart = rf.findEntry(args.PrevLogIndex)

			log.Printf("id:%v,term:%v,tp:%v,appended from %v(term:%v),conflict,firt index for term is:%v\n",
				rf.me, rf.currentTerm, rf.tp, args.LeaderId, args.Term, reply.TermStart)

		}

		log.Printf("id:%v,term:%v,tp:%v,reject appended from %v(term:%v),my log is:%v\n",
			rf.me, rf.currentTerm, rf.tp, args.LeaderId, args.Term, rf.log)

		rf.resetTime()
		rf.mu.Unlock()

		return

	}

	if args.LeaderCommit > rf.commitIndex {

		pre := rf.commitIndex

		if args.LeaderCommit <= len(rf.log)-1 {

			rf.commitIndex = args.LeaderCommit

		} else {

			rf.commitIndex = len(rf.log) - 1

		}

		for i := pre + 1; i <= rf.commitIndex; i++ {

			rf.applych <- ApplyMsg{true, rf.log[i].Command, i}

		}

	}

	rf.resetTime()

	rf.mu.Unlock()

	return

}

func (rf *Raft) resetTime() {
	/*

		离谱的bug
	*/
	rf.timeout = 700 + rand.Intn(400)
	rf.timePassed = 0

}

func (rf *Raft) startElection() {

	rf.mu.Lock()

	log.Printf("id:%v,term:%v,tp:%v,starting an election \n",
		rf.me, rf.currentTerm, rf.tp)

	rf.currentTerm++
	//rf.persist()
	rf.votedFor = rf.me
	//rf.timePassed = 0
	rf.persist()

	rf.tp = 1
	rf.voteCount = 1

	termTemp := rf.currentTerm

	rf.mu.Unlock()

	//var wg sync.WaitGroup

	round := 1
	c := *sync.NewCond(&rf.mu)
	for index, _ := range rf.peers {
		if index == rf.me {
			continue
		}
		//wg.Add(1)

		if rf.killed() {

			return
		}

		go func(server int, round int) {
			//rf.mu.Lock()

			rf.mu.Lock()
			args := &RequestVoteArgs{termTemp, rf.me, len(rf.log) - 1, rf.log[len(rf.log)-1].Term}
			reply := &RequestVoteReply{}
			log.Printf("id:%v,term:%v,tp:%v,sending vote request to %v,round:%v\n",
				rf.me, rf.currentTerm, rf.tp, server, round)

			rf.mu.Unlock()
			ok := rf.sendRequestVote(server, args, reply)

			if rf.killed() {

				return
			}

			log.Printf("id:%v,term:%v,tp:%v,sending vote request to  %v,%v,grand:%v,round:%v\n",
				rf.me, rf.currentTerm, rf.tp, server, ok, reply.VoteGranted, round)
			rf.mu.Lock()
			if rf.currentTerm > termTemp {
			} else if ok && reply.Term > rf.currentTerm {
				rf.termUpdate(reply.Term, 0, -1)
				rf.resetTime()
			} else if ok && reply.VoteGranted == true {
				rf.voteCount++
			}
			// if ok && reply.Term > rf.currentTerm {
			// 	rf.termUpdate(reply.Term, 0, -1)
			// }

			//wg.Done()
			c.Broadcast()
			rf.mu.Unlock()

			return
		}(index, round)
	}

	if rf.killed() {

		return
	}

	//wg.Wait()

	//这个地方是不是改成for循环比较好

	round++

	for rf.currentTerm == termTemp && rf.tp == 1 {

		if rf.killed() {

			return
		}

		// log.Printf("id:%v,term:%v,tp:%v,finish sending a vote request a round,vote%v,round:%v\n",
		// 	rf.me, rf.currentTerm, rf.tp, rf.voteCount, round)
		rf.mu.Lock()
		if rf.voteCount > len(rf.peers)/2 { //此处有race，但不影响

			if rf.killed() {

				return
			}

			rf.tp = 2
			log.Printf("id:%v,term:%v,tp:%v,become a leader\n",
				rf.me, rf.currentTerm, rf.tp)

			for index, _ := range rf.nextIndex {

				rf.nextIndex[index] = len(rf.log)
				rf.matchIndex[index] = 0

			}
			rf.commitCount = make([]int, len(rf.log))

			rf.appC = sync.NewCond(&rf.mu)

			rf.startAppend = false
			for i := 0; i < len(rf.peers); i++ {

				if i != rf.me {

					go func(server int) {

						rf.Append2(server)

					}(i)

				}

			}

			log.Printf("id:%v,term:%v,tp:%v,release lock\n",
				rf.me, rf.currentTerm, rf.tp)
			rf.mu.Unlock()
			return

		}

		//rf.voteCount = 1
		c.Wait()
		rf.mu.Unlock()

		//time.Sleep(time.Duration(60) * time.Millisecond)

	}

	//rf.mu.Unlock()

}

//send append request or heartbeat
func (rf *Raft) Append() {

	log.Printf("id:%v,term:%v,tp:%v,Append\n",
		rf.me, rf.currentTerm, rf.tp)

	termTemp := rf.currentTerm

	for {

		rf.mu.Lock()

		log.Printf("id:%v,term:%v,tp:%v,start append\n",
			rf.me, rf.currentTerm, rf.tp)

		if rf.tp != 2 {

			log.Printf("id:%v,term:%v,tp:%v,give up append\n",
				rf.me, rf.currentTerm, rf.tp)
			// go func() {

			// 	rf.oscillator()

			// }()
			rf.mu.Unlock()
			return
		}

		for index, _ := range rf.peers {

			if index == rf.me {

				continue
			}

			go func(server int) {

				rf.mu.Lock()
				entries := rf.log[rf.nextIndex[server]:] //?
				args := &RequestAppendArgs{rf.currentTerm, rf.me, rf.nextIndex[server] - 1,
					rf.log[rf.nextIndex[server]-1].Term, entries, rf.commitIndex}

				rf.mu.Unlock()

				log.Printf("id:%v,term:%v,tp:%v,sending append request to %v,commitIndex:%v\n",
					rf.me, rf.currentTerm, rf.tp, server, rf.commitIndex)
				reply := &RequestAppendReply{}

				ok := rf.sendAppendEntries(server, args, reply)

				log.Printf("id:%v,term:%v,tp:%v,sending append request to  %v,%v,commitIndex:%v\n",
					rf.me, rf.currentTerm, rf.tp, server, ok, rf.commitIndex)

				if termTemp < rf.currentTerm {

				} else if ok && reply.Term > rf.currentTerm {

					rf.termUpdate(reply.Term, 0, -1)
					rf.resetTime()

					go func() {

						rf.oscillator()

					}()

				} else if ok && reply.Success == false {

					rf.nextIndex[server] = rf.nextIndex[server] - 1

				} else if ok && reply.Success == true {

				}

				return

			}(index)

		}

		rf.mu.Unlock()

		time.Sleep(time.Duration(100) * time.Millisecond)

	}

}

func (rf *Raft) heartbeat() {

	termTemp := rf.currentTerm

	for {

		rf.mu.Lock()

		if rf.tp != 2 {

			log.Printf("id:%v,term:%v,tp:%v,give up heartbeat\n",
				rf.me, rf.currentTerm, rf.tp)
			// go func() {

			// 	rf.oscillator()

			// }()
			rf.mu.Unlock()
			return
		}

		log.Printf("id:%v,term:%v,tp:%v,start heartbeat\n",
			rf.me, rf.currentTerm, rf.tp)

		for index, _ := range rf.peers {

			if index == rf.me {

				continue
			}

			go func(server int) {

				rf.mu.Lock()
				entries := []Log{} //?
				args := &RequestAppendArgs{rf.currentTerm, rf.me, rf.nextIndex[server] - 1,
					rf.log[rf.nextIndex[server]-1].Term, entries, rf.commitIndex}

				rf.mu.Unlock()

				log.Printf("id:%v,term:%v,tp:%v,sending heartbeat to  %v,commitIndex:%v\n",
					rf.me, rf.currentTerm, rf.tp, server, rf.commitIndex)
				reply := &RequestAppendReply{}

				ok := rf.sendAppendEntries(server, args, reply)

				rf.mu.Lock()
				log.Printf("id:%v,term:%v,tp:%v,sending hearbeat to  %v,%v\n",
					rf.me, rf.currentTerm, rf.tp, server, ok)

				if termTemp < rf.currentTerm {

				} else if ok && reply.Term > rf.currentTerm {

					rf.termUpdate(reply.Term, 0, -1)
					rf.resetTime()
					go func() {

						rf.oscillator()

					}()

				}

				rf.mu.Unlock()

				return

			}(index)

		}

		rf.mu.Unlock()

		time.Sleep(time.Duration(100) * time.Millisecond)

	}

}

//one goroutine for a peer
func (rf *Raft) Append2(server int) {

	// log.Printf("id:%v,term:%v,tp:%v,Append to %v\n",
	// 	rf.me, rf.currentTerm, rf.tp, server)

	termTemp := rf.currentTerm

	//flag := false //判定是append还是heartbeat

	//c := sync.NewCond(&rf.mu)
	//done := false

	//finishProbe := false

	count := 0

	for {

		rf.mu.Lock()

		// if rf.nextIndex[server] > len(rf.log)-1 {

		// 	flag = true

		// } else {

		// 	flag = false

		// }

		// log.Printf("id:%v,term:%v,tp:%v,Append to %v\n",
		// 	rf.me, rf.currentTerm, rf.tp, server)

		// log.Printf("id:%v,term:%v,tp:%v,start append\n",
		// 	rf.me, rf.currentTerm, rf.tp)

		if rf.killed() {

			return
		}

		if rf.tp != 2 {

			log.Printf("id:%v,term:%v,tp:%v,give up append\n",
				rf.me, rf.currentTerm, rf.tp)

			rf.mu.Unlock()
			return
		}

		//done := false
		//var wake1 chan time.Time
		wake := make(chan int, 1)
		go func(server int) {

			rf.mu.Lock()

			count++

			entries := rf.log[rf.nextIndex[server]:] //?

			// log.Printf("id:%v,term:%v,tp:%v,to %v,prevLogIndex:%v\n",
			// 	rf.me, rf.currentTerm, rf.tp, server, rf.nextIndex[server]-1)
			args := &RequestAppendArgs{rf.currentTerm, rf.me, rf.nextIndex[server] - 1,
				rf.log[rf.nextIndex[server]-1].Term, entries, rf.commitIndex}

			log.Printf("id:%v,term:%v,tp:%v,sending append request to  %v,%v,commitIndex:%v\n",
				rf.me, rf.currentTerm, rf.tp, server, entries, rf.commitIndex)
			reply := &RequestAppendReply{}
			rf.mu.Unlock()

			countTemp := count
			// go func(cc int) {
			// 	countTemp := cc
			// 	time.Sleep(time.Duration(200) * time.Millisecond)

			// 	if count == countTemp {
			// 		log.Printf("id:%v,term:%v,tp:%v,timeout,%v", rf.me, rf.currentTerm, rf.tp, server)
			// 		c.Broadcast()

			// 	}

			// 	//c.Broadcast()

			// }(count)

			ok := rf.sendAppendEntries(server, args, reply)

			if rf.killed() {

				return
			}

			if count != countTemp {

				return

			}

			if rf.startAppend == false {

				log.Printf("id:%v,term:%v,tp:%v,sending append request to  %v,%v,%v,commitIndex:%v\n",
					rf.me, rf.currentTerm, rf.tp, server, ok, entries, rf.commitIndex)

				return

			}

			rf.mu.Lock()
			if rf.startAppend == false {

				log.Printf("id:%v,term:%v,tp:%v,sending append request to  %v,%v,%v,commitIndex:%v,not ready to append\n",
					rf.me, rf.currentTerm, rf.tp, server, ok, entries, rf.commitIndex)

				rf.mu.Unlock()

				return

			}

			log.Printf("id:%v,term:%v,tp:%v,sending append request to  %v,%v,%v,commitIndex:%v\n",
				rf.me, rf.currentTerm, rf.tp, server, ok, entries, rf.commitIndex)

			// if flag == true {

			// 	rf.mu.Unlock()
			// 	return
			// }

			if termTemp < rf.currentTerm {

			} else if ok && reply.Term > rf.currentTerm {

				rf.termUpdate(reply.Term, 0, -1)
				rf.resetTime()

				go func() {

					rf.oscillator()

				}()

			} else if ok && reply.Success == false {

				if rf.nextIndex[server] == args.PrevLogIndex+1 {

					if reply.Miss == true {

						rf.nextIndex[server] = reply.Len

					} else {

						index := rf.findLast(reply.TermConflict, args.PrevLogIndex)

						if index != -1 {

							rf.nextIndex[server] = index

						} else {

							rf.nextIndex[server] = reply.TermStart
						}

					}

					log.Printf("id:%v,term:%v,tp:%v,sending append request to  %v,commitIndex:%v,nextIndex:%v\n,  pre:%v   %v",
						rf.me, rf.currentTerm, rf.tp, server, rf.commitIndex, rf.nextIndex, rf.nextIndex[server], rf.log)

				}

			} else if ok && reply.Success == true {

				//
				if rf.nextIndex[server] == args.PrevLogIndex+1 && len(args.Entries) > 0 {

					rf.matchIndex[server] = rf.nextIndex[server] - 1 + len(entries)
					rf.nextIndex[server] = rf.nextIndex[server] + len(entries)
					//for i:=args.PrevLogIndex+1;i<=

					rf.updateCommit()

				}

			}

			if len(args.Entries) > 0 {

				if count == countTemp {
					log.Printf("id:%v,term:%v,tp:%v,receive append reply from %v(term:%v) ", rf.me, rf.currentTerm, rf.tp, server, reply.Term)
					//c.Broadcast()

					wake <- 1

				}

			} else {
				log.Printf("id:%v,term:%v,tp:%v,hearbeat ok from %v(term:%v) ", rf.me, rf.currentTerm, rf.tp, server, reply.Term)

			}

			rf.mu.Unlock()

			return

		}(server)

		rf.mu.Unlock()
		t := time.NewTimer(time.Duration(150) * time.Millisecond)

		log.Printf("timer1")
		select {

		case <-t.C:
			log.Printf("id:%v,term:%v,tp:%v,timeout,%v", rf.me, rf.currentTerm, rf.tp, server)
			t.Stop()
			break
		case <-wake:

			t.Stop()
			break

		}
		log.Printf("timer2")

		//c.Wait()

		//time.Sleep(time.Duration(150) * time.Millisecond)

		//rf.mu.Unlock()

	}

}

func (rf *Raft) updateCommit() {

	//rf.mu.Lock()

	// if rf.appendFlag == false {

	// 	rf.appC.Wait()

	// }

	pre := rf.commitIndex

	log.Printf("id:%v,term:%v,tp:%v,updateing commit index,logs:%v,commitIndex,%v,matchIndex,%v\n",
		rf.me, rf.currentTerm, rf.tp, rf.log, pre, rf.matchIndex)

	for i := len(rf.log) - 1; i > rf.commitIndex; i-- {

		count := 0

		for j := 0; j < len(rf.matchIndex); j++ {

			if rf.matchIndex[j] >= i {

				count++

			}

		}

		if count > len(rf.peers)/2 && rf.log[i].Term == rf.currentTerm {

			rf.commitIndex = i

			for k := pre + 1; k <= rf.commitIndex; k++ {

				rf.applych <- ApplyMsg{true, rf.log[k].Command, k}
				log.Printf("id:%v,term:%v,tp:%v,send back log:%v \n",
					rf.me, rf.currentTerm, rf.tp, rf.log[k].Command)
			}

			return

		}

	}

	rf.appendFlag = false

	log.Printf("id:%v,term:%v,tp:%v,new commitIndex %v\n",
		rf.me, rf.currentTerm, rf.tp, rf.commitIndex)

	//rf.mu.Unlock()

}

//find the first entry index of a term
func (rf *Raft) findEntry(p int) int {

	term := rf.log[p].Term

	res := 0

	for rf.log[p].Term == term {

		res = p
		p--

	}

	return res

}

//find the last index of a given term.if the term does not exist,return -1
func (rf *Raft) findLast(t int, start int) int {

	//离谱的bug，之前写的是index:=len(rf.log),这样速度会很慢
	for index := start; index >= 0; index-- {

		if rf.log[index].Term == t {

			return index

		} else if rf.log[index].Term < t {

			return -1
		}

	}

	return 0

}
