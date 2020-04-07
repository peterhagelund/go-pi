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
	defer func() {
		if err != nil {
			unix.Close(fd)
		}
	}()
	capability, err := v4l2.QueryCapabilities(fd)
	if err != nil {
		return nil, err
	}
	fmtDescs, err := v4l2.EnumFormats(fd, v4l2.BufTypeVideoCapture)
	if err != nil {
		return nil, err
	}
	return &camera{
		fd:         fd,
		capability: capability,
		fmtDescs:   fmtDescs,
	}, nil
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
	frameSizeEnums, err := v4l2.EnumFrameSizes(c.fd, pixFormat)
	if err != nil {
		return nil, err
	}
	frameSizes := make([]*FrameSize, 0)
	for _, frameSizeEnum := range frameSizeEnums {
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
	width, height, err = v4l2.SetFormat(c.fd, v4l2.BufTypeVideoCapture, pixFormat, width, height)
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func (c *camera) GrabFrame() ([]byte, error) {
	if c.fd == -1 {
		return nil, syscall.EINVAL
	}
	count, err := v4l2.RequestDriverBuffers(c.fd, 4, v4l2.BufTypeVideoCapture, v4l2.MemoryMmap)
	if err != nil {
		return nil, err
	}
	defer v4l2.RequestDriverBuffers(c.fd, 0, v4l2.BufTypeVideoCapture, v4l2.MemoryMmap)
	buffers, err := v4l2.MmapBuffers(c.fd, count, v4l2.BufTypeVideoCapture)
	if err != nil {
		return nil, err
	}
	defer v4l2.MunmapBuffers(buffers)
	if err := v4l2.StreamOn(c.fd, v4l2.BufTypeVideoCapture); err != nil {
		return nil, err
	}
	defer v4l2.StreamOff(c.fd, v4l2.BufTypeVideoCapture)
	frame, err := v4l2.GrabFrame(c.fd, v4l2.BufTypeVideoCapture, v4l2.MemoryMmap, buffers)
	if err != nil {
		return nil, err
	}
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

func (c *camera) mapFormat(desc string) (v4l2.PixFmt, error) {
	for i := 0; i < len(c.fmtDescs); i++ {
		description := v4l2.BytesToString(c.fmtDescs[i].Description[:])
		if desc == description {
			return c.fmtDescs[i].PixFormat, nil
		}
	}
	return 0, errors.New("unknown format description")
}
