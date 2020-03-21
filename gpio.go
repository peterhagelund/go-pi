package pi

import (
	"io"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Bcm2708Base is the BCM 2708 base address.
const Bcm2708Base int64 = 0x2000000

// GpioOffset is the GPIO offset into the BCM2708 register space.
const GpioOffset int64 = 0x20000

// PageSize (4K) is page size for the BCM 2708 GPIO register space.
const PageSize int = 1 << 12

// GpSet0 is the register number for GPSET0.
const GpSet0 int = 7

// GpClr0 is the register number for GPCLR0.
const GpClr0 int = 10

// GpLev0 is the register number for GPLEV0.
const GpLev0 int = 13

// Pin is the GPIO pin type.
type Pin uint8

const (
	// GPIO2 is GPIO pin 2.
	GPIO2 Pin = iota + 2
	// GPIO3 is GPIO pin 3.
	GPIO3
	// GPIO4 is GPIO pin 4.
	GPIO4
	// GPIO5 is GPIO pin 5.
	GPIO5
	// GPIO6 is GPIO pin 6.
	GPIO6
	// GPIO7 is GPIO pin 7.
	GPIO7
	// GPIO8 is GPIO pin 8.
	GPIO8
	// GPIO9 is GPIO pin 9.
	GPIO9
	// GPIO10 is GPIO pin 10.
	GPIO10
	// GPIO11 is GPIO pin 11.
	GPIO11
	// GPIO12 is GPIO pin 12.
	GPIO12
	// GPIO13 is GPIO pin 13.
	GPIO13
	// GPIO14 is GPIO pin 14.
	GPIO14
	// GPIO15 is GPIO pin 15.
	GPIO15
	// GPIO16 is GPIO pin 16.
	GPIO16
	// GPIO17 is GPIO pin 17.
	GPIO17
	// GPIO18 is GPIO pin 18.
	GPIO18
	// GPIO19 is GPIO pin 19.
	GPIO19
	// GPIO20 is GPIO pin 20.
	GPIO20
	// GPIO21 is GPIO pin 21.
	GPIO21
	// GPIO22 is GPIO pin 22.
	GPIO22
	// GPIO23 is GPIO pin 23.
	GPIO23
	// GPIO24 is GPIO pin 24.
	GPIO24
	// GPIO25 is GPIO pin 25.
	GPIO25
	// GPIO26 is GPIO pin 26.
	GPIO26
	// GPIO27 is GPIO pin 27.
	GPIO27
)

// Direction is the GPIO direction type.
type Direction uint8

const (
	// DirectionInput is the input direction.
	DirectionInput Direction = iota
	// DirectionOutput is the output direction.
	DirectionOutput
)

// Value is the GPIO value type.
type Value uint8

const (
	// ValueOff represents a GPIO voltage equivalent to off.
	ValueOff Value = iota
	// ValueOn represents a GPIO voltage equivalent to on.
	ValueOn
)

// GPIO represents a single GPIO pin.
type GPIO interface {
	io.Closer
	Direction() Direction
	Pin() Pin
	Value() (Value, error)
	SetValue(value Value) error
}

type gpio struct {
	pin       Pin
	direction Direction
	id        uint32
	data      []byte
	registers []uint32
}

// NewGPIO creates and returns a new GPIO instance
func NewGPIO(pin Pin, direction Direction) (GPIO, error) {
	fd, err := unix.Open("/dev/gpiomem", unix.O_RDWR|unix.O_SYNC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	data, err := unix.Mmap(fd, Bcm2708Base+GpioOffset, PageSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	id := uint32(1 << pin)
	registers := *(*[]uint32)(unsafe.Pointer(&data))
	offset := pin / 10
	count := (pin % 10) * 3
	registers[offset] &^= (7 << count)
	if direction == DirectionOutput {
		registers[offset] |= (1 << count)
	}
	return &gpio{
		pin:       pin,
		direction: direction,
		id:        id,
		data:      data,
		registers: registers,
	}, nil
}

func (gpio *gpio) Close() error {
	if gpio.data == nil {
		return syscall.EINVAL
	}
	if err := unix.Munmap(gpio.data); err != nil {
		return err
	}
	gpio.data = nil
	gpio.registers = nil
	return nil
}

func (gpio *gpio) Direction() Direction {
	return gpio.Direction()
}

func (gpio *gpio) Pin() Pin {
	return gpio.pin
}

func (gpio *gpio) Value() (Value, error) {
	if gpio.data == nil {
		return ValueOff, syscall.EINVAL
	}
	var value Value
	if gpio.registers[GpLev0]&gpio.id == 0 {
		value = ValueOff
	} else {
		value = ValueOn
	}
	return value, nil
}

func (gpio *gpio) SetValue(value Value) error {
	if gpio.data == nil {
		return syscall.EINVAL
	}
	if value == ValueOn {
		gpio.registers[GpSet0] = gpio.id
	} else {
		gpio.registers[GpClr0] = gpio.id
	}
	return nil
}
