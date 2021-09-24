package main

import (
	"fmt"
)

func main() {

	a := []int{1, 2, 3, 4, 5, 6}
	b := []int{7, 8}

	fmt.Println(a, len(a), cap(a))
	a = append(a[0:1], b...)

	fmt.Println(a, len(a), cap(a))

}
