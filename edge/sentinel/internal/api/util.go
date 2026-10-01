package api

import "fmt"

func fmtSscan(s string, n *int) (int, error) { return fmt.Sscan(s, n) }
