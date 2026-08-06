package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/innacy/table"
	"github.com/zinscky/log"
)

type FullAttendance struct {
	Data []struct {
		Date            string   `json:"date"`
		Present         []string `json:"present"`
		PresentEmpNames []string `json:"presentEmpNames,omitempty"`
	} `json:"data"`
}

type AttendanceRow struct {
	Date            string   `json:"date,omitempty"`
	PresentList     []string `json:"presentList,omitempty"`
	PresentEmpNames []string `json:"presentEmpNames,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
	UpdatedAt       string   `json:"updatedAt,omitempty"`
}

func Execute(event string, config map[string]string, vars map[string]string, log *log.Logger) (string, error) {
	log.Info("inside attendance store handler")

	fullData := &FullAttendance{}
	err := json.Unmarshal([]byte(event), fullData)
	if err != nil {
		log.Error("failed to unmarshal event: %v", err)
		return "", err
	}

	tableAccessor := table.NewCnipsTableAccessor[AttendanceRow](config["table_base_url"], config["apiKey"])
	tableId := config["tableId"]
	ctx := context.Background()

	for _, dayData := range fullData.Data {
		if len(dayData.Present) == 0 {
			log.Info("skipping date %s: no present employees", dayData.Date)
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)

		query := map[string]any{
			"date": dayData.Date,
		}

		existing, err := tableAccessor.Find(ctx, tableId, query)
		if err != nil {
			log.Error("error checking existing record for date %s: %v", dayData.Date, err)
			return "", err
		}

		if len(existing) > 0 {
			updateData := &AttendanceRow{
				PresentList:     dayData.Present,
				PresentEmpNames: dayData.PresentEmpNames,
				UpdatedAt:       now,
			}
			_, err = tableAccessor.Update(ctx, tableId, query, updateData)
			if err != nil {
				log.Error("error updating record for date %s: %v", dayData.Date, err)
				return "", err
			}
			log.Info("updated attendance for date %s with %d employees", dayData.Date, len(dayData.Present))
		} else {
			row := &AttendanceRow{
				Date:            dayData.Date,
				PresentList:     dayData.Present,
				PresentEmpNames: dayData.PresentEmpNames,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			err = tableAccessor.Insert(ctx, tableId, row)
			if err != nil {
				log.Error("error inserting record for date %s: %v", dayData.Date, err)
				return "", err
			}
			log.Info("inserted attendance for date %s with %d employees", dayData.Date, len(dayData.Present))
		}
	}

	result := map[string]string{
		"message": "attendance store completed",
	}
	marshalResult, _ := json.Marshal(&result)
	return string(marshalResult), nil
}
