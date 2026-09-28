package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

func FetchAttendance(cfg *Config) ([]AttendanceRow, error) {
	connURL := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(cfg.DBUser, cfg.DBPassword),
		Host:   fmt.Sprintf("%s:%d", cfg.DBHost, cfg.DBPort),
	}
	q := connURL.Query()
	q.Set("database", cfg.DBName)
	q.Set("connection timeout", "30")
	connURL.RawQuery = q.Encode()

	db, err := sql.Open("sqlserver", connURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	now := time.Now()
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := todayMidnight.AddDate(0, 0, -cfg.DaysToFetch) // Fetch last N days of attendance

	query := `
		SELECT DISTINCT e.EmployeeCode, e.EmployeeName, CAST(a.AttendanceDate AS DATE) AS AttendanceDate
		FROM AttendanceLogs a INNER JOIN Employees e ON a.EmployeeId = e.EmployeeId
		WHERE a.AttendanceDate >= @startDate
		  AND a.AttendanceDate < @todayDate
		  AND a.Status IN ('Present', '1/2Present', '1/2 Present')
		ORDER BY CAST(a.AttendanceDate AS DATE), e.EmployeeCode
	`

	rows, err := db.QueryContext(ctx, query,
		sql.Named("startDate", startDate),
		sql.Named("todayDate", todayMidnight),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var result []AttendanceRow
	for rows.Next() {
		var row AttendanceRow
		if err := rows.Scan(&row.BadgeNumber, &row.EmployeeName, &row.CheckTime); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}
