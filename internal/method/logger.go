package method

import "fmt"

func DefaultLogger(Type string, log ...any) {
	fmt.Printf("[%s] %s\n", Type, fmt.Sprint(log...))
}
