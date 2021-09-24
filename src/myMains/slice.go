package main

import "fmt"

func main() {

	a := make([]int, 5)

	c := make([]int, 2)
	c[0] = 1
	c[1] = 2

	b := a[5:]
	fmt.Printf("%p\n", a)
	fmt.Printf("%p\n", b)
	fmt.Println(len(b))
	fmt.Println(len(a))
	fmt.Println(cap(a))
	fmt.Printf("%p\n", c)

	fmt.Println(a)

}
