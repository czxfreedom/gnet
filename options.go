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

package gnet

import (
	"time"

	"github.com/panjf2000/gnet/pkg/logging"
)

// Option is a function that will set up option.
type Option func(opts *Options)

func loadOptions(options ...Option) *Options {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	return opts
}

// TCPSocketOpt is the type of TCP socket options.
type TCPSocketOpt int

// Available TCP socket options.
const (
	TCPNoDelay TCPSocketOpt = iota
	TCPDelay
)

// Options are configurations for the gnet application.
type Options struct {
	// ================================== Options for only server-side ==================================

	// Multicore indicates whether the server will be effectively created with multi-cores, if so,
	// then you must take care with synchronizing memory between all event callbacks, otherwise,
	// it will run the server with single thread. The number of threads in the server will be automatically
	// assigned to the value of logical CPUs usable by the current process.
	//Multicore表示是否使用多核有效创建服务器，如果是，
	//然后必须注意在所有事件回调之间同步内存，否则，
	//它将使用单线程运行服务器。服务器中的线程数将自动更改
	//分配给当前进程可用的逻辑CPU的值。
	Multicore bool

	// NumEventLoop is set up to start the given number of event-loop goroutine.
	// Note: Setting up NumEventLoop will override Multicore.
	//NumEventLoop设置为启动给定数量的事件循环goroutine。
	//注意：设置NumEventLoop将覆盖多核。
	NumEventLoop int

	// LB represents the load-balancing algorithm used when assigning new connections.
	//LB表示分配新连接时使用的负载平衡算法。
	LB LoadBalancing

	// ReuseAddr indicates whether to set up the SO_REUSEADDR socket option.
	////ReuseAddr指示是否设置SO_ReuseAddr套接字选项。
	ReuseAddr bool

	// ReusePort indicates whether to set up the SO_REUSEPORT socket option.
	//重用端口
	ReusePort bool

	// ============================= Options for both server-side and client-side =============================

	// ReadBufferCap is the maximum number of bytes that can be read from the peer when the readable event comes.
	// The default value is 64KB, it can be reduced to avoid starving the subsequent connections.
	//
	// Note that ReadBufferCap will always be converted to the least power of two integer value greater than
	// or equal to its real amount.

	//请注意，ReadBufferCap将始终转换为大于的两个整数值的最小幂
	//或等于其实际数值。
	ReadBufferCap int

	// LockOSThread is used to determine whether each I/O event-loop is associated to an OS thread, it is useful when you
	// need some kind of mechanisms like thread local storage, or invoke certain C libraries (such as graphics lib: GLib)
	// that require thread-level manipulation via cgo, or want all I/O event-loops to actually run in parallel for a
	// potential higher performance.
	//LockOSThread用于确定每个I/O事件循环是否与OS线程相关联，在
	//需要某种机制，如线程本地存储，或调用某些C库（如graphics lib:GLib）
	//需要通过cgo进行线程级操作，或者希望所有I/O事件循环实际并行运行一段时间
	//潜在的更高的性能。
	LockOSThread bool

	// Ticker indicates whether the ticker has been set up.
	//Ticker指示是否已设置该Ticker。
	Ticker bool

	// TCPKeepAlive sets up a duration for (SO_KEEPALIVE) socket option.
	TCPKeepAlive time.Duration

	// TCPNoDelay controls whether the operating system should delay
	// packet transmission in hopes of sending fewer packets (Nagle's algorithm).
	//
	// The default is true (no delay), meaning that data is sent
	// as soon as possible after a write operation.
	////TCPNoDelay控制操作系统是否应延迟
	////希望发送更少数据包的数据包传输（Nagle算法）。
	////默认值为true（无延迟），表示发送数据
	////在写操作后尽快。
	TCPNoDelay TCPSocketOpt

	// SocketRecvBuffer sets the maximum socket receive buffer in bytes.
	SocketRecvBuffer int

	// SocketSendBuffer sets the maximum socket send buffer in bytes.
	SocketSendBuffer int

	// ICodec encodes and decodes TCP stream.
	Codec ICodec

	// LogPath the local path where logs will be written, this is the easiest way to set up logging,
	// gnet instantiates a default uber-go/zap logger with this given log path, you are also allowed to employ
	// you own logger during the lifetime by implementing the following log.Logger interface.
	//
	// Note that this option can be overridden by the option Logger.
	//LogPath将写入日志的本地路径，这是设置日志的最简单方法，
	//gnet用这个给定的日志路径实例化一个默认的uber go/zap记录器，您也可以使用
	//通过实现以下日志，您可以在生命周期内拥有记录器。记录器接口。
	//请注意，此选项可由选项记录器覆盖。
	LogPath string

	// LogLevel indicates the logging level, it should be used along with LogPath.
	LogLevel logging.Level

	// Logger is the customized logger for logging info, if it is not set,
	// then gnet will use the default logger powered by go.uber.org/zap.
	Logger logging.Logger
}

// WithOptions sets up all options.
func WithOptions(options Options) Option {
	return func(opts *Options) {
		*opts = options
	}
}

// WithMulticore sets up multi-cores in gnet server.
func WithMulticore(multicore bool) Option {
	return func(opts *Options) {
		opts.Multicore = multicore
	}
}

// WithLockOSThread sets up LockOSThread mode for I/O event-loops.
func WithLockOSThread(lockOSThread bool) Option {
	return func(opts *Options) {
		opts.LockOSThread = lockOSThread
	}
}

// WithReadBufferCap sets up ReadBufferCap for reading bytes.
func WithReadBufferCap(readBufferCap int) Option {
	return func(opts *Options) {
		opts.ReadBufferCap = readBufferCap
	}
}

// WithLoadBalancing sets up the load-balancing algorithm in gnet server.
func WithLoadBalancing(lb LoadBalancing) Option {
	return func(opts *Options) {
		opts.LB = lb
	}
}

// WithNumEventLoop sets up NumEventLoop in gnet server.
func WithNumEventLoop(numEventLoop int) Option {
	return func(opts *Options) {
		opts.NumEventLoop = numEventLoop
	}
}

// WithReusePort sets up SO_REUSEPORT socket option.
func WithReusePort(reusePort bool) Option {
	return func(opts *Options) {
		opts.ReusePort = reusePort
	}
}

// WithReuseAddr sets up SO_REUSEADDR socket option.
func WithReuseAddr(reuseAddr bool) Option {
	return func(opts *Options) {
		opts.ReuseAddr = reuseAddr
	}
}

// WithTCPKeepAlive sets up the SO_KEEPALIVE socket option with duration.
func WithTCPKeepAlive(tcpKeepAlive time.Duration) Option {
	return func(opts *Options) {
		opts.TCPKeepAlive = tcpKeepAlive
	}
}

// WithTCPNoDelay enable/disable the TCP_NODELAY socket option.
func WithTCPNoDelay(tcpNoDelay TCPSocketOpt) Option {
	return func(opts *Options) {
		opts.TCPNoDelay = tcpNoDelay
	}
}

// WithSocketRecvBuffer sets the maximum socket receive buffer in bytes.
func WithSocketRecvBuffer(recvBuf int) Option {
	return func(opts *Options) {
		opts.SocketRecvBuffer = recvBuf
	}
}

// WithSocketSendBuffer sets the maximum socket send buffer in bytes.
func WithSocketSendBuffer(sendBuf int) Option {
	return func(opts *Options) {
		opts.SocketSendBuffer = sendBuf
	}
}

// WithTicker indicates that a ticker is set.
func WithTicker(ticker bool) Option {
	return func(opts *Options) {
		opts.Ticker = ticker
	}
}

// WithCodec sets up a codec to handle TCP stream.
func WithCodec(codec ICodec) Option {
	return func(opts *Options) {
		opts.Codec = codec
	}
}

// WithLogPath is an option to set up the local path of log file.
func WithLogPath(fileName string) Option {
	return func(opts *Options) {
		opts.LogPath = fileName
	}
}

// WithLogLevel is an option to set up the logging level.
func WithLogLevel(lvl logging.Level) Option {
	return func(opts *Options) {
		opts.LogLevel = lvl
	}
}

// WithLogger sets up a customized logger.
func WithLogger(logger logging.Logger) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}
