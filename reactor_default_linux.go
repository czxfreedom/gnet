// Copyright (c) 2019 Andy Pan
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

//go:build !poll_opt
// +build !poll_opt

package gnet

import (
	"runtime"

	"github.com/panjf2000/gnet/internal/netpoll"
	"github.com/panjf2000/gnet/pkg/errors"
)

func (el *eventloop) activateMainReactor(lockOSThread bool) {
	if lockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	defer el.svr.signalShutdown()

	err := el.poller.Polling(func(fd int, ev uint32) error { return el.svr.accept(fd, ev) })
	if err == errors.ErrServerShutdown {
		el.svr.opts.Logger.Debugf("main reactor is exiting in terms of the demand from user, %v", err)
	} else if err != nil {
		el.svr.opts.Logger.Errorf("main reactor is exiting due to error: %v", err)
	}
}

func (el *eventloop) activateSubReactor(lockOSThread bool) {
	if lockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	defer func() {
		el.closeAllSockets()
		el.svr.signalShutdown()
	}()

	err := el.poller.Polling(func(fd int, ev uint32) error {
		if c, ack := el.connections[fd]; ack {
			// Don't change the ordering of processing EPOLLOUT | EPOLLRDHUP / EPOLLIN unless you're 100%
			// sure what you're doing!
			// Re-ordering can easily introduce bugs and bad side-effects, as I found out painfully in the past.

			// We should always check for the EPOLLOUT event first, as we must try to send the leftover data back to
			// the peer when any error occurs on a connection.
			//
			// Either an EPOLLOUT or EPOLLERR event may be fired when a connection is refused.
			// In either case write() should take care of it properly:
			// 1) writing data back,
			// 2) closing the connection.
			if ev&netpoll.OutEvents != 0 && !c.outboundBuffer.IsEmpty() {
				if err := el.write(c); err != nil {
					return err
				}
			}
			// If there is pending data in outbound buffer, then we should omit this readable event
			// and prioritize the writable events to achieve a higher performance.
			//
			// Note that the peer may send massive amounts of data to server by write() under blocking mode,
			// resulting in that it won't receive any responses before the server reads all data from the peer,
			// in which case if the server socket send buffer is full, we need to let it go and continue reading
			// the data to prevent blocking forever.
			if ev&netpoll.InEvents != 0 && (ev&netpoll.OutEvents == 0 || c.outboundBuffer.IsEmpty()) {
				return el.read(c)
			}
		}
		return nil
	})
	if err == errors.ErrServerShutdown {
		el.svr.opts.Logger.Debugf("event-loop(%d) is exiting in terms of the demand from user, %v", el.idx, err)
	} else if err != nil {
		el.svr.opts.Logger.Errorf("event-loop(%d) is exiting due to error: %v", el.idx, err)
	}
}

func (el *eventloop) run(lockOSThread bool) {
	if lockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	defer func() {
		el.closeAllSockets()
		el.ln.close()
		el.svr.signalShutdown()
	}()

	err := el.poller.Polling(func(fd int, ev uint32) error {
		if c, ok := el.connections[fd]; ok {
			// Don't change the ordering of processing EPOLLOUT | EPOLLRDHUP / EPOLLIN unless you're 100%
			// sure what you're doing!
			// Re-ordering can easily introduce bugs and bad side-effects, as I found out painfully in the past.

			// We should always check for the EPOLLOUT event first, as we must try to send the leftover data back to
			// the peer when any error occurs on a connection.
			//
			// Either an EPOLLOUT or EPOLLERR event may be fired when a connection is refused.
			// In either case write() should take care of it properly:
			// 1) writing data back,
			// 2) closing the connection.
			//除非您达到100%，否则不要更改处理EPOLLOUT | EPOLLDHUP/EPOLLIN的顺序
			//当然知道你在做什么！
			//正如我在过去痛苦地发现的那样，重新排序很容易引入错误和不良副作用。
			//我们应该始终首先检查ePlolout事件，因为我们必须尝试将剩余数据发送回服务器
			//当连接上发生任何错误时，对等方将被删除
			//当连接被拒绝时，可能会触发EPOLLOUT或EPOLLERR事件。
			//在任何一种情况下，write（）都应该正确处理它：
			//1）回写数据，
			//2）关闭连接。
			if ev&netpoll.OutEvents != 0 && !c.outboundBuffer.IsEmpty() {
				if err := el.write(c); err != nil {
					return err
				}
			}
			// If there is pending data in outbound buffer, then we should omit this readable event
			// and prioritize the writable events to achieve a higher performance.
			//
			// Note that the peer may send massive amounts of data to server by write() under blocking mode,
			// resulting in that it won't receive any responses before the server reads all data from the peer,
			// in which case if the socket send buffer is full, we need to let it go and continue reading the data
			// to prevent blocking forever.
			////若出站缓冲区中有挂起的数据，那个么我们应该忽略这个可读事件
			////并对可写事件进行优先级排序，以实现更高的性能。
			////注意，在阻塞模式下，对等方可能通过write（）向服务器发送大量数据，
			////导致在服务器从对等方读取所有数据之前，它不会收到任何响应，
			////在这种情况下，如果套接字发送缓冲区已满，我们需要释放它并继续读取数据
			////防止永远堵塞。
			if ev&netpoll.InEvents != 0 && (ev&netpoll.OutEvents == 0 || c.outboundBuffer.IsEmpty()) {
				return el.read(c)
			}
			return nil
		}
		return el.accept(fd, ev)
	})
	el.getLogger().Debugf("event-loop(%d) is exiting due to error: %v", el.idx, err)
}
