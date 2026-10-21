-- name: RecordJobRun :exec
INSERT INTO job_runs (job, last_run_at, last_result)
VALUES (@job, @last_run_at, @last_result)
ON CONFLICT (job) DO UPDATE SET last_run_at = EXCLUDED.last_run_at, last_result = EXCLUDED.last_result;

-- name: GetJobRun :one
SELECT * FROM job_runs WHERE job = @job;
