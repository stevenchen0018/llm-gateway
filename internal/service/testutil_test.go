package service

import "time"

func timeAt(sec int64) time.Time       { return time.Unix(1_700_000_000+sec, 0) }
func hoursDur(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }
