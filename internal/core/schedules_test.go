// 스케줄의 시간대·중복 슬롯·겹침·중단 누락 정책을 검증한다.
package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScheduleValidationDueSlotsAndOverlap(t *testing.T) {
	c := newClient(t)
	c.login("admin")
	seedPlugin(t, c.s, helperExecutable(t))
	for _, body := range []string{
		`{"name":"bad","plugin_id":"hello","task_id":"greet","cron":"* * * * * *","timezone":"UTC","input":{},"enabled":true}`,
		`{"name":"bad","plugin_id":"hello","task_id":"greet","cron":"* * * * *","timezone":"Local","input":{},"enabled":true}`,
		`{"name":"bad","plugin_id":"hello","task_id":"greet","cron":"* * * * *","timezone":"Not/AZone","input":{},"enabled":true}`,
	} {
		status(t, c.request("POST", "/api/v1/schedules", body), 400)
	}
	w := c.request("POST", "/api/v1/schedules", `{"name":"Every minute","plugin_id":"hello","task_id":"greet","cron":"* * * * *","timezone":"Asia/Seoul","input":{},"enabled":true}`)
	status(t, w, 201)
	var schedule Schedule
	if e := json.Unmarshal(w.Body.Bytes(), &schedule); e != nil {
		t.Fatal(e)
	}
	next, e := time.Parse(time.RFC3339Nano, schedule.NextRunAt)
	if e != nil || !next.After(time.Now()) {
		t.Fatalf("next not calculated %v %v", next, e)
	}
	due := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, e = c.s.db.Exec("UPDATE schedules SET next_run_at=? WHERE id=?", due.Format(timestampFormat), schedule.ID); e != nil {
		t.Fatal(e)
	}
	if e = c.s.processDueSchedules(due); e != nil {
		t.Fatal(e)
	}
	if e = c.s.processDueSchedules(due); e != nil {
		t.Fatal(e)
	}
	var count int
	c.s.db.QueryRow("SELECT count(*) FROM runs WHERE schedule_id=?", schedule.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("duplicate/missing due slot runs=%d", count)
	}
	if e = c.s.processDueSchedules(due.Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	c.s.db.QueryRow("SELECT count(*) FROM runs WHERE schedule_id=?", schedule.ID).Scan(&count)
	if count != 1 {
		t.Fatal("overlap accumulated work")
	}
	var outcome string
	c.s.db.QueryRow("SELECT outcome FROM schedule_slots WHERE schedule_id=? AND scheduled_at=?", schedule.ID, due.Add(time.Minute).Format(timestampFormat)).Scan(&outcome)
	if outcome != "overlap_skipped" {
		t.Fatalf("no overlap history %q", outcome)
	}
	if _, e = c.s.db.Exec("UPDATE schedules SET next_run_at=? WHERE id=?", time.Now().Add(-24*time.Hour).UTC().Format(timestampFormat), schedule.ID); e != nil {
		t.Fatal(e)
	}
	if e = c.s.StartBackground(); e != nil {
		t.Fatal(e)
	}
	var at string
	c.s.db.QueryRow("SELECT next_run_at FROM schedules WHERE id=?", schedule.ID).Scan(&at)
	next, e = time.Parse(time.RFC3339Nano, at)
	if e != nil || !next.After(time.Now()) {
		t.Fatalf("missed slots not skipped %s %v", at, e)
	}
	status(t, c.request("DELETE", "/api/v1/schedules/"+schedule.ID, ""), 204)
}
func TestCronIANAAndDST(t *testing.T) {
	before := time.Date(2030, 3, 9, 12, 0, 0, 0, time.UTC)
	next, e := nextSchedule("30 2 * * *", "America/New_York", before)
	// 2030-03-10 02:30 does not exist, so the next valid local date is March 11.
	if e != nil || next.Format(time.RFC3339) != "2030-03-11T06:30:00Z" {
		t.Fatalf("DST gap was not respected %v %v", next, e)
	}
	next, e = nextSchedule("0 9 * * 1", "Asia/Seoul", time.Date(2030, 1, 6, 23, 59, 0, 0, time.UTC))
	if e != nil || next.Format(time.RFC3339) != "2030-01-07T00:00:00Z" {
		t.Fatalf("IANA schedule wrong %v %v", next, e)
	}
}
