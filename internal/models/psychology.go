package models

import "gorm.io/gorm"

type Psychology struct {
	gorm.Model
	ID        uint `gorm:"primaryKey" json:"id"`
	JournalID uint `gorm:"uniqueIndex" json:"journalId"`

	Emotion        int `json:"emotion"`
	Confidence     int `json:"confidence"`
	ExecutionScore int `json:"executionScore"`
	Discipline     int `json:"discipline"`

	PsyTags string `json:"psyTags"` // CSV

	FollowedPlan       bool `json:"followedPlan"`
	RespectedRisk      bool `json:"respectedRisk"`
	WaitedConfirmation bool `json:"waitedConfirmation"`
	NoRevenge          bool `json:"noRevenge"`
}
