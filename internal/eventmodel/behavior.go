package eventmodel

import (
	"strings"
)

type Behavior string

const (
	BehaviorProcessExec    Behavior = "process.exec"
	BehaviorProcessFork    Behavior = "process.fork"
	BehaviorProcessExit    Behavior = "process.exit"
	BehaviorFileOpen       Behavior = "file.open"
	BehaviorFileRead       Behavior = "file.read"
	BehaviorFileWrite      Behavior = "file.write"
	BehaviorFileChmod      Behavior = "file.chmod"
	BehaviorNetworkConnect Behavior = "network.connect"
)

var AllBehaviors = []Behavior{
	BehaviorProcessExec,
	BehaviorProcessFork,
	BehaviorProcessExit,
	BehaviorFileOpen,
	BehaviorFileRead,
	BehaviorFileWrite,
	BehaviorFileChmod,
	BehaviorNetworkConnect,
}

func (b Behavior) String() string { return string(b) }

func KnownBehavior(name string) bool {
	switch NormalizeBehavior(name) {
	case BehaviorProcessExec, BehaviorProcessFork, BehaviorProcessExit, BehaviorFileOpen, BehaviorFileRead, BehaviorFileWrite, BehaviorFileChmod, BehaviorNetworkConnect:
		return true
	default:
		return false
	}
}

func NormalizeBehavior(name string) Behavior {
	return Behavior(strings.TrimSpace(strings.ToLower(name)))
}

type CapabilityMatrix struct {
	Backend     string
	Behavior    Behavior
	Fields      []string
	Pushdown    []string
	AgentSide   []string
	Unsupported []string
}
