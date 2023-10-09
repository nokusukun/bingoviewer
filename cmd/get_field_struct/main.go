package main

import (
	"fmt"
	"github.com/nokusukun/bingo"
)

type Empty map[string]any

func (Empty) Key() []byte {
	return nil
}

func main() {
	driver, err := bingo.NewDriver(bingo.DriverConfiguration{
		Filename: "opaws.db",
	})
	if err != nil {
		panic(err)
	}

	users := bingo.CollectionFrom[Empty](driver, "users")
	docs, err := users.Find(func(doc Empty) bool {
		return true
	})
	if err != nil {
		panic(err)
	}
	for _, doc := range docs {
		fmt.Println(doc)
	}
}
