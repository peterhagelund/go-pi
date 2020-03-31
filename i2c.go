// Copyright (c) 2020 Peter Hagelund
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package pi

import (
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	// I2CRetries specifies the number of times a device should be polled when not acknowledging.
	I2CRetries uint32 = 0x00000701 + iota
	// I2CTimeout specifies the timeout in units of 10 ms.
	I2CTimeout
	// I2CSlave specifies the use this slave address.
	I2CSlave
	// I2CTenBit sets the use of ten-bit addresses (0 == 7 bits; not 0 == 10 bits).
	I2CTenBit
	// I2CFuncs gets the adapter functionality mask.
	I2CFuncs
	// I2CSlaveForce specifies the use of this slave address even if it's already in use by a driver.
	I2CSlaveForce
	// I2CRdWr specifieds the use of a combined read/write )on stop only).
	I2CRdWr
	// I2CPEC enables packet error checking.
	I2CPEC
)

// I2CBus defines the behavior of an I2C bus).
type I2CBus interface {
	io.Reader
	io.Writer
	io.Closer
	// SetSlave addresses a specific slave.
	SetSlave(address uint8) error
}

type i2cBus struct {
	fd int
}

// OpenI2CBus opens the I2C bus at the specified path.
func OpenI2CBus(path string) (I2CBus, error) {
	fd, err := unix.Open(path, unix.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &i2cBus{
		fd: fd,
	}, nil
}

func (i *i2cBus) Read(p []byte) (n int, err error) {
	n = 0
	err = nil
	if i.fd == -1 {
		err = syscall.EINVAL
		return
	}
	n, err = unix.Read(i.fd, p)
	return
}

func (i *i2cBus) Write(p []byte) (n int, err error) {
	n = 0
	err = nil
	if i.fd == -1 {
		err = syscall.EINVAL
		return
	}
	n, err = unix.Write(i.fd, p)
	return
}

func (i *i2cBus) Close() error {
	if i.fd == -1 {
		return syscall.EINVAL
	}
	if err := unix.Close(i.fd); err != nil {
		return err
	}
	i.fd = -1
	return nil
}

func (i *i2cBus) SetSlave(address uint8) error {
	if i.fd == -1 {
		return syscall.EINVAL
	}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(i.fd), uintptr(I2CSlave), uintptr(address)); err != 0 {
		return err
	}
	return nil
}
