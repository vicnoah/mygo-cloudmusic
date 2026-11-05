package dlna

import (
	"context"
	"testing"
	"time"
)

// Tests controller initialization, device selection state, and cleanup.
func TestControllerState(t *testing.T) {
	statusCalled := false
	ctrl, err := NewController(func() {
		statusCalled = true
	})
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer ctrl.Close()

	if ctrl.IsCasting() {
		t.Error("expected IsCasting() to be false initially")
	}

	// Mocking devices list
	mockDev := &Device{
		ID:           "http://192.168.1.50:1400/xml/desc.xml",
		FriendlyName: "Living Room Speaker",
		ModelName:    "HiFi-DLNA",
		URL:          "http://192.168.1.50:1400/xml/desc.xml",
	}
	ctrl.mu.Lock()
	ctrl.devices = []*Device{mockDev}
	ctrl.mu.Unlock()

	// Select device
	if err := ctrl.SelectDevice(mockDev.ID); err != nil {
		t.Fatalf("SelectDevice: %v", err)
	}

	if !ctrl.IsCasting() {
		t.Error("expected IsCasting() to be true after selecting device")
	}
	if !statusCalled {
		t.Error("expected status callback to be invoked on device selection")
	}

	active := ctrl.ActiveDevice()
	if active == nil || active.FriendlyName != "Living Room Speaker" {
		t.Errorf("got active device %+v, want Living Room Speaker", active)
	}

	// Test volume
	_ = ctrl.SetVolume(context.Background(), 0.65)
	if ctrl.volume != 0.65 {
		t.Errorf("got volume %v, want 0.65", ctrl.volume)
	}

	// Deselect (switch back to local)
	statusCalled = false
	if err := ctrl.SelectDevice(""); err != nil {
		t.Fatalf("SelectDevice(''): %v", err)
	}
	if ctrl.IsCasting() {
		t.Error("expected IsCasting() to be false after deselecting")
	}
	if !statusCalled {
		t.Error("expected status callback when deselecting device")
	}
}

// Tests parseDuration helper for UPnP RelTime strings.
func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"0:03:45", 3*time.Minute + 45*time.Second},
		{"1:15:30", 1*time.Hour + 15*time.Minute + 30*time.Second},
		{"0:00:12.500", 12*time.Second + 500*time.Millisecond},
		{"120", 2 * time.Minute},
		{"invalid", 0},
	}
	for _, tt := range tests {
		got := parseDuration(tt.input)
		if got != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// Tests that an active device is never dropped from the device list when a scan cycle runs.
func TestActiveDeviceNeverDisappearsOnRefresh(t *testing.T) {
	ctrl, err := NewController(nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	defer ctrl.Close()

	mockDev := &Device{
		ID:           "http://192.168.1.50:1400/xml/desc.xml",
		FriendlyName: "Living Room Speaker",
		ModelName:    "HiFi-DLNA",
		URL:          "http://192.168.1.50:1400/xml/desc.xml",
	}

	ctrl.mu.Lock()
	ctrl.knownDevices[mockDev.ID] = mockDev
	ctrl.rebuildDeviceListLocked()
	ctrl.mu.Unlock()

	if err := ctrl.SelectDevice(mockDev.ID); err != nil {
		t.Fatalf("SelectDevice: %v", err)
	}

	if ctrl.ActiveDevice() == nil {
		t.Fatal("expected active device to be set")
	}

	// Simulate rebuild without new SSDP responses
	ctrl.mu.Lock()
	ctrl.rebuildDeviceListLocked()
	ctrl.mu.Unlock()

	devs := ctrl.Devices()
	found := false
	for _, d := range devs {
		if d.ID == mockDev.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected active device to still be present in Devices() list")
	}
	if ctrl.ActiveDevice() == nil || ctrl.ActiveDevice().ID != mockDev.ID {
		t.Error("expected active device to remain selected")
	}
}
