package mr

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"time"
)

type Master struct {
	// Your definitions here.

	mu   sync.Mutex
	cond *sync.Cond

	MapTasks    []string
	ReduceTasks []int //储存reduce task每一部分都是由
	//哪个workder生成的。方便传递目录

	MapStatus    []bool //每个task完成的状态，一个task可能包含多个file
	ReduceStatus []bool

	MapFinished    int //map task未完成的数目
	ReduceFinished int //reduce task未完成的数目

	Nreduce int

	MapHead    *TaskNode
	ReduceHead *TaskNode

	MapPointer    *TaskNode //对task进行分配时使用的指针
	ReducePointer *TaskNode

	workers int //worker的数量
}

type TaskNode struct {
	taskId int
	next   *TaskNode
	prev   *TaskNode

	count int //被访问的次数
}

// Your code here -- RPC handlers for the worker to call.

type Pg struct {
	name     string
	finished bool
}

func (m *Master) RequestHandler(args *StateArgs, reply *StateReply) error {

	m.mu.Lock()

	reply.WorkerId = args.WorkerId
	if args.Finished {

		if args.Cat == 1 && !m.MapStatus[args.TaskId] {

			m.MapStatus[args.TaskId] = true
			m.MapFinished--

			m.ReduceTasks[args.TaskId] = args.WorkerId
			//reply.WorkerId = args.WorkerId

		} else if args.Cat == 2 && !m.ReduceStatus[args.TaskId] {

			m.ReduceStatus[args.TaskId] = true
			m.ReduceFinished--
			os.Rename(args.FileName, "mr-out-"+strconv.Itoa(args.TaskId))
			//reply.WorkerId = args.WorkerId

		}

	}

	if args.Idle {

		if args.WorkerId == 0 {

			reply.WorkerId = m.workers + 1
			m.workers++
		}

		m.assignTask(reply)

	}

	m.mu.Unlock()

	return nil
}

func (m *Master) assignTask(reply *StateReply) {

	//time.Sleep(100 * time.Millisecond)

	if m.MapFinished > 0 {

		reply.Cat = 1 //assign a map task

		//找到一个未完成的task并删除已经完成的task

		task := m.MapPointer
		for ; ; task = task.next {

			if task == m.MapHead {

			} else if m.MapStatus[task.taskId] {

				DeleteNode(task)
			} else {
				break
			}

		}

		reply.TaskId = task.taskId
		reply.FileName = m.MapTasks[task.taskId]
		reply.Nreduce = m.Nreduce
		m.MapPointer = task.next

	} else if m.ReduceFinished > 0 {

		reply.Cat = 2

		task := m.ReducePointer
		for ; ; task = task.next {

			if task == m.ReduceHead { //一开始我写成了MapHead

				//此时已经将task分配了一圈，等待
				//一段时间，让worker执行任务
				time.Sleep(50 * time.Millisecond)

				//m.cond.Wait()

			} else if m.ReduceStatus[task.taskId] {

				DeleteNode(task)
			} else {
				break
			}

		}

		reply.TaskId = task.taskId
		reply.Nreduce = m.Nreduce
		reply.MapCount = len(m.MapStatus)
		reply.FileNames = m.ReduceTasks
		m.MapPointer = task.next

	} else {

		reply.Cat = 0

	}

}

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

//
// start a thread that listens for RPCs from worker.go
//
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

//
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	ret := false

	// Your code here.

	m.mu.Lock()
	if m.MapFinished == 0 && m.ReduceFinished == 0 {

		ret = true

		//os.RemoveAll("../../lab_interFiles")

	}
	m.mu.Unlock()

	return ret
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//

func DeleteNode(node *TaskNode) {

	temp := node.prev
	temp.next = node.next
	node.next.prev = temp

}
func AddNode(head *TaskNode, node *TaskNode) {

	temp := head.prev

	temp.next = node
	node.prev = temp
	node.next = head
	head.prev = node

}

func InitHead(head *TaskNode) {

	head.next = head
	head.prev = head
	head.taskId = -1

}

func MakeMaster(files []string, nReduce int) *Master {
	m := Master{}

	// Your code here.

	m.MapTasks = files
	m.MapFinished = len(files)
	m.Nreduce = nReduce
	m.ReduceFinished = nReduce
	m.MapStatus = make([]bool, len(files))
	m.ReduceStatus = make([]bool, nReduce)
	m.MapHead = new(TaskNode)
	m.ReduceHead = new(TaskNode)

	m.ReduceTasks = make([]int, len(files))

	InitHead(m.MapHead)
	InitHead(m.ReduceHead)

	for i := 0; i < len(files); i++ {

		AddNode(m.MapHead, &TaskNode{taskId: i})

	}

	for i := 0; i < nReduce; i++ {

		AddNode(m.ReduceHead, &TaskNode{taskId: i})
	}

	m.MapPointer = m.MapHead.next
	m.ReducePointer = m.ReduceHead.next

	m.cond = sync.NewCond(&m.mu) //为什么会传一个pointer

	os.Mkdir("lab_interFiles", 0755)

	m.server()
	return &m
}
