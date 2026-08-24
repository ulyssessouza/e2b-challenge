package main

import "e2b-challenge/internal/config"

func main() {
	cfg := config.Load()
	_ = cfg
}