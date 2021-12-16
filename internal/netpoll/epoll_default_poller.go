// Copyright (c) 2019 Andy Pan
// Copyright (c) 2017 Joshua J Baker
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux && !poll_opt
// +build linux,!poll_opt

package netpoll

import (
	"os"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/panjf2000/gnet/internal/queue"
	"github.com/panjf2000/gnet/pkg/errors"
	"github.com/panjf2000/gnet/pkg/logging"
)

// Poller represents a poller which is in charge of monitoring file-descriptors.
type Poller struct {
	fd                  int    // epoll fd
	wfd                 int    // wake fd 唤醒的文件描述符
	wfdBuf              []byte // wfd buffer to read packet 用于读取数据包的wfd缓冲区
	netpollWakeSig      int32
	asyncTaskQueue      queue.AsyncTaskQueue // queue with low priority  低优先级队列
	priorAsyncTaskQueue queue.AsyncTaskQueue // queue with high priority 高优先级队列
}

// OpenPoller instantiates a poller.
func OpenPoller() (poller *Poller, err error) {
	poller = new(Poller)
	if poller.fd, err = unix.EpollCreate1(unix.EPOLL_CLOEXEC); err != nil {
		poller = nil
		err = os.NewSyscallError("epoll_create1", err)
		return
	}
	if poller.wfd, err = unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC); err != nil {
		_ = poller.Close()
		poller = nil
		err = os.NewSyscallError("eventfd", err)
		return
	}
	poller.wfdBuf = make([]byte, 8)
	if err = poller.AddRead(&PollAttachment{FD: poller.wfd}); err != nil {
		_ = poller.Close()
		poller = nil
		return
	}
	poller.asyncTaskQueue = queue.NewLockFreeQueue()
	poller.priorAsyncTaskQueue = queue.NewLockFreeQueue()
	return
}

// Close closes the poller.
func (p *Poller) Close() error {
	if err := os.NewSyscallError("close", unix.Close(p.fd)); err != nil {
		return err
	}
	return os.NewSyscallError("close", unix.Close(p.wfd))
}

// Make the endianness of bytes compatible with more linux OSs under different processor-architectures,
// according to http://man7.org/linux/man-pages/man2/eventfd.2.html.
var (
	u uint64 = 1
	b        = (*(*[8]byte)(unsafe.Pointer(&u)))[:]
)

// UrgentTrigger puts task into priorAsyncTaskQueue and wakes up the poller which is waiting for network-events,
// then the poller will get tasks from priorAsyncTaskQueue and run them.
//
// Note that priorAsyncTaskQueue is a queue with high-priority and its size is expected to be small,
// so only those urgent tasks should be put into this queue.
////UrgentTrigger将任务放入priorAsyncTaskQueue并唤醒正在等待网络事件的轮询器，
////然后轮询器将从priorAsyncTaskQueue获取任务并运行它们。
////请注意，priorAsyncTaskQueue是一个具有高优先级的队列，其大小预计较小，
////因此，只有那些紧急任务才应放入此队列。
func (p *Poller) UrgentTrigger(fn queue.TaskFunc, arg interface{}) (err error) {
	task := queue.GetTask()
	task.Run, task.Arg = fn, arg
	//客户端有新的请求过来了,将任务入队
	p.priorAsyncTaskQueue.Enqueue(task)
	//往 nfd写一个唤醒消息
	if atomic.CompareAndSwapInt32(&p.netpollWakeSig, 0, 1) {
		for _, err = unix.Write(p.wfd, b); err == unix.EINTR || err == unix.EAGAIN; _, err = unix.Write(p.wfd, b) {
		}
	}
	return os.NewSyscallError("write", err)
}

// Trigger is like UrgentTrigger but it puts task into asyncTaskQueue,
// call this method when the task is not so urgent, for instance writing data back to the peer.
//
// Note that asyncTaskQueue is a queue with low-priority whose size may grow large and tasks in it may backlog.
//触发器类似于UrgentTrigger，但它将任务放入asyncTaskQueue，
//当任务不是很紧急时调用此方法，例如将数据写回对等方。
//请注意，asyncTaskQueue是一个低优先级队列，其大小可能会变大，其中的任务可能会积压。
func (p *Poller) Trigger(fn queue.TaskFunc, arg interface{}) (err error) {
	task := queue.GetTask()
	task.Run, task.Arg = fn, arg
	p.asyncTaskQueue.Enqueue(task)
	if atomic.CompareAndSwapInt32(&p.netpollWakeSig, 0, 1) {
		for _, err = unix.Write(p.wfd, b); err == unix.EINTR || err == unix.EAGAIN; _, err = unix.Write(p.wfd, b) {
		}
	}
	return os.NewSyscallError("write", err)
}

// Polling blocks the current goroutine, waiting for network-events.
////轮询阻塞当前goroutine，等待网络事件。
func (p *Poller) Polling(callback func(fd int, ev uint32) error) error {
	el := newEventList(InitPollEventsCap)
	var wakenUp bool

	msec := -1
	for {
		n, err := unix.EpollWait(p.fd, el.events, msec)
		if n == 0 || (n < 0 && err == unix.EINTR) {
			msec = -1
			runtime.Gosched()
			continue
		} else if err != nil {
			logging.Errorf("error occurs in epoll: %v", os.NewSyscallError("epoll_wait", err))
			return err
		}
		msec = 0

		for i := 0; i < n; i++ {
			ev := &el.events[i]
			//对于主reacotr 有新的链接 就调用 accept, 子reactor 只会接受到 主reactor发的wakeUp信号
			if fd := int(ev.Fd); fd != p.wfd {
				switch err = callback(fd, ev.Events); err {
				case nil:
				case errors.ErrAcceptSocket, errors.ErrServerShutdown:
					return err
				default:
					logging.Warnf("error occurs in event-loop: %v", err)
				}
			} else { // poller is awaken to run tasks in queues.
				//轮询器被唤醒以在队列中运行任务。
				wakenUp = true
				_, _ = unix.Read(p.wfd, p.wfdBuf)
			}
		}
		//接到唤醒的信号
		if wakenUp {
			wakenUp = false
			task := p.priorAsyncTaskQueue.Dequeue()
			//紧急任务出队
			for ; task != nil; task = p.priorAsyncTaskQueue.Dequeue() {
				//运行任务
				switch err = task.Run(task.Arg); err {
				case nil:
				case errors.ErrServerShutdown:
					return err
				default:
					logging.Warnf("error occurs in user-defined function, %v", err)
				}
				//任务清空,然后放回池子里  池化
				queue.PutTask(task)
			}
			//普通任务出队
			for i := 0; i < MaxAsyncTasksAtOneTime; i++ {
				if task = p.asyncTaskQueue.Dequeue(); task == nil {
					break
				}
				switch err = task.Run(task.Arg); err {
				case nil:
				case errors.ErrServerShutdown:
					return err
				default:
					logging.Warnf("error occurs in user-defined function, %v", err)
				}
				queue.PutTask(task)
			}
			atomic.StoreInt32(&p.netpollWakeSig, 0)
			//唤醒信号置为1
			if (!p.asyncTaskQueue.IsEmpty() || !p.priorAsyncTaskQueue.IsEmpty()) && atomic.CompareAndSwapInt32(&p.netpollWakeSig, 0, 1) {
				for _, err = unix.Write(p.wfd, b); err == unix.EINTR || err == unix.EAGAIN; _, err = unix.Write(p.wfd, b) {
				}
			}
		}

		if n == el.size {
			//事件队列扩容
			el.expand()
		} else if n < el.size>>1 {
			//事件队列缩容
			el.shrink()
		}
	}
}

const (
	readEvents      = unix.EPOLLPRI | unix.EPOLLIN
	writeEvents     = unix.EPOLLOUT
	readWriteEvents = readEvents | writeEvents
)

// AddReadWrite registers the given file-descriptor with readable and writable events to the poller.
func (p *Poller) AddReadWrite(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl add",
		unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readWriteEvents}))
}

// AddRead registers the given file-descriptor with readable event to the poller.
func (p *Poller) AddRead(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl add",
		unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readEvents}))
}

// AddWrite registers the given file-descriptor with writable event to the poller.
func (p *Poller) AddWrite(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl add",
		unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: writeEvents}))
}

// ModRead renews the given file-descriptor with readable event in the poller.
func (p *Poller) ModRead(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl mod",
		unix.EpollCtl(p.fd, unix.EPOLL_CTL_MOD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readEvents}))
}

// ModReadWrite renews the given file-descriptor with readable and writable events in the poller.
func (p *Poller) ModReadWrite(pa *PollAttachment) error {
	return os.NewSyscallError("epoll_ctl mod",
		unix.EpollCtl(p.fd, unix.EPOLL_CTL_MOD, pa.FD, &unix.EpollEvent{Fd: int32(pa.FD), Events: readWriteEvents}))
}

// Delete removes the given file-descriptor from the poller.
func (p *Poller) Delete(fd int) error {
	return os.NewSyscallError("epoll_ctl del", unix.EpollCtl(p.fd, unix.EPOLL_CTL_DEL, fd, nil))
}
