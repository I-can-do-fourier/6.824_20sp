package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"../mr"
)

func main() {

	pairs := []mr.KeyValue{}

	path := "../lab_interFiles/mr-0-0"
	file, err := os.Open(path)

	if err != nil {
		log.Fatalf("cannot open %v", file)
	}

	var er error

	for er != io.EOF && er == nil {

		var k string
		var v string
		_, er = fmt.Fscanf(file, "%s %s\n", &k, &v)

		pairs = append(pairs, mr.KeyValue{Key: k, Value: v})
	}

	pairs = pairs[0 : len(pairs)-1] //删掉最后一个空的

	for _, item := range pairs {

		fmt.Println(item)

	}

	//fmt.Println("haha")

}
