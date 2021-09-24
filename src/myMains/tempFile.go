package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
)

func main() {

	dic := "../lab_interFiles"
	randomString := "mr-" + strconv.Itoa(1) + "-" + strconv.Itoa(2)
	//f, err := os.Create(path)
	f, err := ioutil.TempFile(dic, randomString)
	if err != nil {
		panic(err)
	}

	fmt.Println(f.Name())

	//defer os.Remove(f.Name())

	//fmt.Println("haha")

}
