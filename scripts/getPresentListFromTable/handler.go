package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/innacy/table"
	"github.com/zinscky/log"
)

type AttendanceRecord struct {
	Date        string   `json:"date,omitempty"`
	PresentList []string `json:"presentList,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

func Execute(event string, config map[string]string, vars map[string]string, log *log.Logger) (string, error) {
	log.Info("inside attendance get handler")

	tableAccessor := table.NewCnipsTableAccessor[AttendanceRecord](config["table_base_url"], config["apiKey"])
	tableId := config["tableId"]
	ctx := context.Background()

	daysToFetch, err := strconv.Atoi(config["days_to_fetch"])
	if err != nil {
		log.Error("invalid days_to_fetch value: %v", err)
		return "", err
	}

	today := time.Now().UTC()
	weekdays := getLastWeekWeekdays(today, daysToFetch)

	log.Info("fetching attendance for %d weekdays", len(weekdays))

	var results []AttendanceRecord

	for _, date := range weekdays {
		dateStr := date.Format("2006-01-02")

		query := map[string]any{
			"date": dateStr,
		}

		rows, err := tableAccessor.Find(ctx, tableId, query)
		if err != nil {
			log.Error("error fetching record for date %s: %v", dateStr, err)
			return "", err
		}

		if len(rows) > 0 {
			results = append(results, rows[0])
			log.Info("found record for %s with %d present", dateStr, len(rows[0].PresentList))
		} else {
			log.Info("no record found for %s", dateStr)
		}
	}

	log.Info("total records fetched: %d", len(results))

	var outputEvent map[string]interface{}
	if err := json.Unmarshal([]byte(event), &outputEvent); err != nil {
		log.Error("failed to unmarshal input event: %v", err)
		return "", err
	}
	outputEvent["presentList"] = results

	output, err := json.Marshal(outputEvent)
	if err != nil {
		log.Error("error marshalling results: %v", err)
		return "", err
	}

	return string(output), nil
}

func getLastWeekWeekdays(today time.Time, daysToFetch int) []time.Time {
	var weekdays []time.Time

	for i := 1; i <= daysToFetch; i++ {
		day := today.AddDate(0, 0, -i)
		wd := day.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			continue
		}
		weekdays = append(weekdays, day)
	}

	return weekdays
}
