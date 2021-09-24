package main

import (
	"fmt"
	"sync"

	"../mr"
	"../myTests"
	"../haha"
)

func main() {

	mm:=haha.ha{}

	fmt.Println(mm)

	a := 1
	myTests.M()
	//haha := labrpc.reqMs{}
	//fmt.Print(haha)
	t := mr.Master{}
	fmt.Println(t)
	var mu sync.Mutex

	mu.Lock()

	if a == 1 {

		fmt.Println(a)
		return
		mu.Unlock()
	} else {

		fmt.Println(a - 1)
		return

	}

	a = 4
	a = 5
	d := 6
	fmt.Println(d)

}
