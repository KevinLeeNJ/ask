package stream

import "github.com/KevinLeeNJ/ask/internal/domain/provider"

type ProgressStage string

const (
	StagePreparing  ProgressStage = "preparing"
	StageConnecting ProgressStage = "connecting"
	StageWaiting    ProgressStage = "waiting"
	StageThinking   ProgressStage = "thinking"
	StageGenerating ProgressStage = "generating"
	StageComplete   ProgressStage = "complete"
)

type Progress struct {
	Stage     ProgressStage
	ElapsedMs int64
}

type Event interface {
	isEvent()
}

type TextDelta struct {
	Text string
}

func (TextDelta) isEvent() {}

type ReasoningDelta struct {
	Text string
}

func (ReasoningDelta) isEvent() {}

type Usage struct {
	Usage provider.Usage
}

func (Usage) isEvent() {}

type Completed struct{}

func (Completed) isEvent() {}

type Failed struct {
	Err error
}

func (Failed) isEvent() {}
