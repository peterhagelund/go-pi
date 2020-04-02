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
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // Import to register JPEG
	_ "image/png"  // Import to register PNG
	"io"
	"syscall"
	"unsafe"

	"github.com/peterhagelund/go-v4l2"
	"golang.org/x/sys/unix"
)

// FrameSize is an encapsulation of a support frame size or steps.
type FrameSize struct {
	// IsDiscreet indicates whether or not this frame size is discreet.
	IsDiscreet bool
	// IsStepwise indicates whether or not this frame size is stepwise.
	IsStepwise bool
	// IsContinuous indicates whether or not this frame size is continuous.
	IsContinuous bool
	// Width is the discreet width.
	Width uint32
	// Height is the discreet height.
	Height uint32
	// MinWidth is the minimum width.
	MinWidth uint32
	// MaxWidth is the maximum width.
	MaxWidth uint32
	// StepWidth is the step width.
	StepWidth uint32
	// MinHeight is the minimum height.
	MinHeight uint32
	// MaxHeight is the maximum height.
	MaxHeight uint32
	// StepHeight is the step height.
	StepHeight uint32
}

// Camera defines the behavior of a Camera.
type Camera interface {
	io.Closer
	// Driver returns the camera driver name.
	Driver() (string, error)
	// Card rturns the camera card name.
	Card() (string, error)
	// BusInfo returns the camera bus info.
	BusInfo() (string, error)
	// Formats returns the supported formats.
	Formats() ([]string, error)
	// FrameSizes returns the supported frame sizes for a given format.
	FrameSizes(desc string) ([]*FrameSize, error)
	// SetFormat sets the format and frame size.
	SetFormat(desc string, width uint32, height uint32) (uint32, uint32, error)
	// GrabFrame grabs a single frame.
	GrabFrame() ([]byte, error)
	// GrabImage grabs a single frame and returns it as an image.
	GrabImage() (image.Image, string, error)
}

type camera struct {
	fd         int
	capability *v4l2.Capability
	fmtDescs   []*v4l2.FmtDesc
}

// OpenCamera opens the camera device at the specified path.
func OpenCamera(path string) (Camera, error) {
	fd, err := unix.Open(path, unix.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	c := &camera{
		fd: fd,
	}
	defer func() {
		if err != nil {
			c.Close()
		}
	}()
	err = c.queryCapabilities()
	if err != nil {
		return nil, err
	}
	err = c.enumFormats()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *camera) Close() error {
	if c.fd == -1 {
		return syscall.EINVAL
	}
	if err := unix.Close(c.fd); err != nil {
		return err
	}
	c.fd = -1
	c.capability = nil
	c.fmtDescs = nil
	return nil
}

func (c *camera) Driver() (string, error) {
	if c.fd == -1 {
		return "", syscall.EINVAL
	}
	return v4l2.BytesToString(c.capability.Driver[:]), nil
}

func (c *camera) Card() (string, error) {
	if c.fd == -1 {
		return "", syscall.EINVAL
	}
	return v4l2.BytesToString(c.capability.Card[:]), nil
}

func (c *camera) BusInfo() (string, error) {
	if c.fd == -1 {
		return "", syscall.EINVAL
	}
	return v4l2.BytesToString(c.capability.BusInfo[:]), nil
}

func (c *camera) Formats() ([]string, error) {
	if c.fd == -1 {
		return nil, syscall.EINVAL
	}
	formats := make([]string, 0, len(c.fmtDescs))
	for i := 0; i < len(c.fmtDescs); i++ {
		formats = append(formats, v4l2.BytesToString(c.fmtDescs[i].Description[:]))
	}
	return formats, nil
}

func (c *camera) FrameSizes(desc string) ([]*FrameSize, error) {
	if c.fd == -1 {
		return nil, syscall.EINVAL
	}
	pixFormat, err := c.mapFormat(desc)
	if err != nil {
		return nil, err
	}
	frameSizes := make([]*FrameSize, 0)
	frameSizeEnum := &v4l2.FrameSizeEnum{}
	frameSizeEnum.PixFormat = pixFormat
	for {
		err := v4l2.Ioctl(c.fd, v4l2.VidIocEnumFrameSizes, uintptr(unsafe.Pointer(frameSizeEnum)))
		if err != nil {
			if err == syscall.EINVAL {
				break
			}
			return nil, err
		}
		frameSize := &FrameSize{}
		if frameSizeEnum.Type == v4l2.FrmSizeTypeDiscrete {
			discrete := (*v4l2.FrameSizeDiscrete)(unsafe.Pointer(&frameSizeEnum.M))
			frameSize.IsDiscreet = true
			frameSize.Width = discrete.Width
			frameSize.Height = discrete.Height
		} else {
			stepwise := (*v4l2.FrameSizeStepwise)(unsafe.Pointer(&frameSizeEnum.M))
			if frameSizeEnum.Type == v4l2.FrmSizeTypeStepwise {
				frameSize.IsStepwise = true
			} else {
				frameSize.IsContinuous = true
			}
			frameSize.MinWidth = stepwise.MinWidth
			frameSize.MaxWidth = stepwise.MaxWidth
			frameSize.StepWidth = stepwise.StepWidth
			frameSize.MinHeight = stepwise.MinHeight
			frameSize.MaxHeight = stepwise.MaxHeight
			frameSize.StepHeight = stepwise.StepHeight
		}
		frameSizes = append(frameSizes, frameSize)
		frameSizeEnum.Index++
	}
	return frameSizes, nil
}

func (c *camera) SetFormat(desc string, width uint32, height uint32) (uint32, uint32, error) {
	if c.fd == -1 {
		return 0, 0, syscall.EINVAL
	}
	pixFormat, err := c.mapFormat(desc)
	if err != nil {
		return 0, 0, err
	}
	format := &v4l2.Format{}
	format.Type = v4l2.BufTypeVideoCapture
	pix := (*v4l2.PixFormat)(unsafe.Pointer(&format.RawData[0]))
	pix.Width = width
	pix.Height = height
	pix.PixFormat = pixFormat
	pix.Field = v4l2.FieldNone
	if err := v4l2.Ioctl(c.fd, v4l2.VidIocSFmt, uintptr(unsafe.Pointer(format))); err != nil {
		return 0, 0, err
	}
	return pix.Width, pix.Height, nil
}

func (c *camera) GrabFrame() ([]byte, error) {
	if c.fd == -1 {
		return nil, syscall.EINVAL
	}
	requestBuffers := &v4l2.RequestBuffers{}
	requestBuffers.Count = 4
	requestBuffers.Type = v4l2.BufTypeVideoCapture
	requestBuffers.Memory = v4l2.MemoryMMap
	if err := v4l2.Ioctl(c.fd, v4l2.VidIocReqBufs, uintptr(unsafe.Pointer(requestBuffers))); err != nil {
		return nil, err
	}
	buffers := make([][]byte, 0)
	var index uint32
	for index = 0; index < requestBuffers.Count; index++ {
		buffer := &v4l2.Buffer{}
		buffer.Index = index
		buffer.Type = requestBuffers.Type
		buffer.Memory = requestBuffers.Memory
		if err := v4l2.Ioctl(c.fd, v4l2.VidIocQueryBuf, uintptr(unsafe.Pointer(buffer))); err != nil {
			return nil, err
		}
		offset := int64(buffer.M)
		length := int(buffer.Length)
		data, err := unix.Mmap(c.fd, offset, length, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
		if err != nil {
			return nil, err
		}
		defer unix.Munmap(data)
		buffers = append(buffers, data)
		if err := v4l2.Ioctl(c.fd, v4l2.VidIocQBuf, uintptr(unsafe.Pointer(buffer))); err != nil {
			return nil, err
		}
	}
	if err := v4l2.Ioctl(c.fd, v4l2.VidIocStreamOn, uintptr(unsafe.Pointer(&requestBuffers.Type))); err != nil {
		return nil, err
	}
	defer v4l2.Ioctl(c.fd, v4l2.VidIocStreamOff, uintptr(unsafe.Pointer(&requestBuffers.Type)))
	if err := v4l2.WaitFd(c.fd); err != nil {
		return nil, err
	}
	buffer := &v4l2.Buffer{}
	buffer.Type = requestBuffers.Type
	buffer.Memory = requestBuffers.Memory
	if err := v4l2.Ioctl(c.fd, v4l2.VidIocDQBuf, uintptr(unsafe.Pointer(buffer))); err != nil {
		return nil, err
	}
	data := buffers[buffer.Index]
	frame := make([]byte, buffer.BytesUsed)
	copy(frame, data)
	return frame, nil
}

func (c *camera) GrabImage() (image.Image, string, error) {
	frame, err := c.GrabFrame()
	if err != nil {
		return nil, "", err
	}
	buffer := bytes.NewBuffer(frame)
	image, name, err := image.Decode(buffer)
	if err != nil {
		return nil, "", err
	}
	return image, name, nil
}

func (c *camera) queryCapabilities() error {
	capability := &v4l2.Capability{}
	if err := v4l2.Ioctl(c.fd, v4l2.VidIocQueryCap, uintptr(unsafe.Pointer(capability))); err != nil {
		return err
	}
	c.capability = capability
	return nil
}

func (c *camera) enumFormats() error {
	var index uint32 = 0
	formats := make([]*v4l2.FmtDesc, 0, 8)
	for {
		fmtDesc := &v4l2.FmtDesc{}
		fmtDesc.Index = index
		fmtDesc.Type = v4l2.BufTypeVideoCapture
		err := v4l2.Ioctl(c.fd, v4l2.VidIocEnumFmt, uintptr(unsafe.Pointer(fmtDesc)))
		if err != nil {
			if err == syscall.EINVAL {
				break
			}
			return err
		}
		formats = append(formats, fmtDesc)
		index++
	}
	c.fmtDescs = formats
	return nil
}

func (c *camera) mapFormat(desc string) (v4l2.PixFmt, error) {
	for i := 0; i < len(c.fmtDescs); i++ {
		description := v4l2.BytesToString(c.fmtDescs[i].Description[:])
		if desc == description {
			return c.fmtDescs[i].PixFormat, nil
		}
	}
	return 0, errors.New("unknown format description")
}
