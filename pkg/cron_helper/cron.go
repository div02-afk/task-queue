package cron_helper

import (
	"time"

	"github.com/robfig/cron/v3"
)

func GetNextRunAt(cronExpr string) (time.Time, error) {
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, err
	}
	nextRunAt := schedule.Next(time.Now())
	return nextRunAt, nil

}
