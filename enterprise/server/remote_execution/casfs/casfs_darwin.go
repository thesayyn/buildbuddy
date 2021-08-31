// +build darwin

package casfs

import (
	"context"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
)

func (n *Node) CopyFileRange(ctx context.Context, fhIn fs.FileHandle, offIn uint64, out *fs.Inode, fhOut fs.FileHandle, offOut uint64, len uint64, flags uint64) (uint32, syscall.Errno) {
	return 0, syscall.ENOSYS
}