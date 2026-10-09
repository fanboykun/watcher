package agent

import "testing"

func TestPollingMonitorLifecycleAndGeneration(t *testing.T) {
	m := NewPollingMonitor()
	if activity, _ := m.Snapshot(1, false); activity != "stopped" {
		t.Fatal(activity)
	}
	first := m.register(1)
	if activity, _ := m.Snapshot(1, false); activity != "active" {
		t.Fatal(activity)
	}
	m.checking(1, first, true)
	if activity, started := m.Snapshot(1, false); activity != "checking" || started == nil {
		t.Fatalf("%s %v", activity, started)
	}
	m.checking(1, first, false)
	if activity, _ := m.Snapshot(1, true); activity != "paused" {
		t.Fatal(activity)
	}
	second := m.register(1)
	m.checking(1, first, true)
	m.stop(1, first)
	if activity, _ := m.Snapshot(1, false); activity != "active" {
		t.Fatal("old generation modified live loop:", activity)
	}
	m.stop(1, second)
	if activity, _ := m.Snapshot(1, false); activity != "stopped" {
		t.Fatal(activity)
	}
}
