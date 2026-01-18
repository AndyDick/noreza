package output

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

// Extended mouse button codes
const (
	btnLeft    = 0x110
	btnRight   = 0x111
	btnMiddle  = 0x112
	btnSide    = 0x113
	btnExtra   = 0x114
	btnForward = 0x115
	btnBack    = 0x116
)

// uinput constants
const (
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
	uiSetEvBit   = 0x40045564
	uiSetKeyBit  = 0x40045565
	uiSetRelBit  = 0x40045566

	evKey = 0x01
	evRel = 0x02
	evSyn = 0x00

	relX      = 0x00
	relY      = 0x01
	relWheel  = 0x08
	relHWheel = 0x06

	busUSB = 0x03

	uinputMaxNameSize = 80
)

type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputUserDev struct {
	Name       [uinputMaxNameSize]byte
	ID         inputID
	EffectsMax uint32
	Absmax     [64]int32
	Absmin     [64]int32
	Absfuzz    [64]int32
	Absflat    [64]int32
}

type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

// ExtMouse is a mouse device with extra button support
type ExtMouse struct {
	deviceFile *os.File
}

// CreateExtMouse creates a mouse device with back/forward button support
func CreateExtMouse(path string, name []byte) (*ExtMouse, error) {
	deviceFile, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		return nil, fmt.Errorf("could not open %s: %v", path, err)
	}

	// Register EV_KEY
	if err := ioctl(deviceFile, uiSetEvBit, uintptr(evKey)); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to register key events: %v", err)
	}

	// Register all mouse buttons including back/forward
	for _, btn := range []int{btnLeft, btnRight, btnMiddle, btnSide, btnExtra, btnForward, btnBack} {
		if err := ioctl(deviceFile, uiSetKeyBit, uintptr(btn)); err != nil {
			deviceFile.Close()
			return nil, fmt.Errorf("failed to register button %x: %v", btn, err)
		}
	}

	// Register EV_REL
	if err := ioctl(deviceFile, uiSetEvBit, uintptr(evRel)); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to register relative events: %v", err)
	}

	// Register relative axes
	for _, rel := range []int{relX, relY, relWheel, relHWheel} {
		if err := ioctl(deviceFile, uiSetRelBit, uintptr(rel)); err != nil {
			deviceFile.Close()
			return nil, fmt.Errorf("failed to register relative axis %d: %v", rel, err)
		}
	}

	// Create the device
	var devName [uinputMaxNameSize]byte
	copy(devName[:], name)

	uidev := uinputUserDev{
		Name: devName,
		ID: inputID{
			Bustype: busUSB,
			Vendor:  0x4711,
			Product: 0x0817,
			Version: 1,
		},
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, uidev); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to serialize device info: %v", err)
	}

	if _, err := deviceFile.Write(buf.Bytes()); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to write device info: %v", err)
	}

	if err := ioctl(deviceFile, uiDevCreate, uintptr(0)); err != nil {
		deviceFile.Close()
		return nil, fmt.Errorf("failed to create device: %v", err)
	}

	return &ExtMouse{deviceFile: deviceFile}, nil
}

func (m *ExtMouse) Close() error {
	if err := ioctl(m.deviceFile, uiDevDestroy, uintptr(0)); err != nil {
		return fmt.Errorf("failed to destroy device: %v", err)
	}
	return m.deviceFile.Close()
}

func (m *ExtMouse) sendEvent(evType, code uint16, value int32) error {
	ev := inputEvent{
		Time:  syscall.Timeval{Sec: 0, Usec: 0},
		Type:  evType,
		Code:  code,
		Value: value,
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, ev); err != nil {
		return err
	}

	if _, err := m.deviceFile.Write(buf.Bytes()); err != nil {
		return err
	}

	return m.sync()
}

func (m *ExtMouse) sync() error {
	ev := inputEvent{
		Time:  syscall.Timeval{Sec: 0, Usec: 0},
		Type:  evSyn,
		Code:  0,
		Value: 0,
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, ev); err != nil {
		return err
	}

	_, err := m.deviceFile.Write(buf.Bytes())
	return err
}

func (m *ExtMouse) ButtonPress(code int) error {
	return m.sendEvent(evKey, uint16(code), 1)
}

func (m *ExtMouse) ButtonRelease(code int) error {
	return m.sendEvent(evKey, uint16(code), 0)
}

func (m *ExtMouse) Wheel(horizontal bool, delta int32) error {
	code := relWheel
	if horizontal {
		code = relHWheel
	}
	return m.sendEvent(evRel, uint16(code), delta)
}

// Convenience methods matching uinput.Mouse interface
func (m *ExtMouse) LeftPress() error    { return m.ButtonPress(btnLeft) }
func (m *ExtMouse) LeftRelease() error  { return m.ButtonRelease(btnLeft) }
func (m *ExtMouse) RightPress() error   { return m.ButtonPress(btnRight) }
func (m *ExtMouse) RightRelease() error { return m.ButtonRelease(btnRight) }
func (m *ExtMouse) MiddlePress() error   { return m.ButtonPress(btnMiddle) }
func (m *ExtMouse) MiddleRelease() error { return m.ButtonRelease(btnMiddle) }
func (m *ExtMouse) BackPress() error     { return m.ButtonPress(btnBack) }
func (m *ExtMouse) BackRelease() error   { return m.ButtonRelease(btnBack) }
func (m *ExtMouse) ForwardPress() error   { return m.ButtonPress(btnForward) }
func (m *ExtMouse) ForwardRelease() error { return m.ButtonRelease(btnForward) }

func ioctl(deviceFile *os.File, request, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, deviceFile.Fd(), request, arg)
	if errno != 0 {
		return fmt.Errorf("ioctl failed: %v", errno)
	}
	return nil
}
