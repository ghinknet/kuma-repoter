package method

import "fmt"

func DefaultLogger(Type string, log ...interface{}) {
	fmt.Printf("[%s] %s\n", Type, fmt.Sprint(log...))
}
