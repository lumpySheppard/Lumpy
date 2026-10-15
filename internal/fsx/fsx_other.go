//go:build !darwin && !linux

package fsx

import "os"

func noCache(f *os.File) {}

func dropCache(f *os.File) {}
