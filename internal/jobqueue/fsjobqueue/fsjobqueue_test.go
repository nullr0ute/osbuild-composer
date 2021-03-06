package fsjobqueue_test

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/osbuild/osbuild-composer/internal/jobqueue"
	"github.com/osbuild/osbuild-composer/internal/jobqueue/fsjobqueue"
)

type testResult struct {
}

func cleanupTempDir(t *testing.T, dir string) {
	err := os.RemoveAll(dir)
	require.NoError(t, err)
}

func newTemporaryQueue(t *testing.T, jobTypes []string) (jobqueue.JobQueue, string) {
	dir, err := ioutil.TempDir("", "jobqueue-test-")
	require.NoError(t, err)

	q, err := fsjobqueue.New(dir, jobTypes)
	require.NoError(t, err)
	require.NotNil(t, q)

	return q, dir
}

func pushTestJob(t *testing.T, q jobqueue.JobQueue, jobType string, args interface{}, dependencies []uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := q.Enqueue(jobType, args, dependencies)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	return id
}

func finishNextTestJob(t *testing.T, q jobqueue.JobQueue, jobType string, result interface{}) uuid.UUID {
	id, err := q.Dequeue(context.Background(), []string{jobType}, &json.RawMessage{})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	err = q.FinishJob(id, result)
	require.NoError(t, err)

	return id
}

func TestNonExistant(t *testing.T) {
	q, err := fsjobqueue.New("/non-existant-directory", []string{})
	require.Error(t, err)
	require.Nil(t, q)
}

func TestErrors(t *testing.T) {
	q, dir := newTemporaryQueue(t, []string{"test"})
	defer cleanupTempDir(t, dir)

	// not serializable to JSON
	id, err := q.Enqueue("test", make(chan string), nil)
	require.Error(t, err)
	require.Equal(t, uuid.Nil, id)

	// invalid dependency
	id, err = q.Enqueue("test", "arg0", []uuid.UUID{uuid.New()})
	require.Error(t, err)
	require.Equal(t, uuid.Nil, id)
}

func TestArgs(t *testing.T) {
	type argument struct {
		I int
		S string
	}

	q, dir := newTemporaryQueue(t, []string{"fish", "octopus"})
	defer cleanupTempDir(t, dir)

	oneargs := argument{7, "🐠"}
	one := pushTestJob(t, q, "fish", oneargs, nil)

	twoargs := argument{42, "🐙"}
	two := pushTestJob(t, q, "octopus", twoargs, nil)

	var args argument
	id, err := q.Dequeue(context.Background(), []string{"octopus"}, &args)
	require.NoError(t, err)
	require.Equal(t, two, id)
	require.Equal(t, twoargs, args)

	id, err = q.Dequeue(context.Background(), []string{"fish"}, &args)
	require.NoError(t, err)
	require.Equal(t, one, id)
	require.Equal(t, oneargs, args)
}

func TestJobTypes(t *testing.T) {
	q, dir := newTemporaryQueue(t, []string{"octopus", "clownfish"})
	defer cleanupTempDir(t, dir)

	one := pushTestJob(t, q, "octopus", nil, nil)
	two := pushTestJob(t, q, "clownfish", nil, nil)

	require.Equal(t, two, finishNextTestJob(t, q, "clownfish", testResult{}))
	require.Equal(t, one, finishNextTestJob(t, q, "octopus", testResult{}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	id, err := q.Dequeue(ctx, []string{"zebra"}, nil)
	require.Equal(t, err, context.Canceled)
	require.Equal(t, uuid.Nil, id)
}

func TestDependencies(t *testing.T) {
	q, dir := newTemporaryQueue(t, []string{"test"})
	defer cleanupTempDir(t, dir)

	t.Run("done-before-pushing-dependant", func(t *testing.T) {
		one := pushTestJob(t, q, "test", nil, nil)
		two := pushTestJob(t, q, "test", nil, nil)

		r := []uuid.UUID{}
		r = append(r, finishNextTestJob(t, q, "test", testResult{}))
		r = append(r, finishNextTestJob(t, q, "test", testResult{}))
		require.ElementsMatch(t, []uuid.UUID{one, two}, r)

		j := pushTestJob(t, q, "test", nil, []uuid.UUID{one, two})
		queued, started, finished, canceled, err := q.JobStatus(j, nil)
		require.NoError(t, err)
		require.True(t, !queued.IsZero())
		require.True(t, started.IsZero())
		require.True(t, finished.IsZero())
		require.False(t, canceled)

		require.Equal(t, j, finishNextTestJob(t, q, "test", testResult{}))

		queued, started, finished, canceled, err = q.JobStatus(j, &testResult{})
		require.NoError(t, err)
		require.True(t, !queued.IsZero())
		require.True(t, !started.IsZero())
		require.True(t, !finished.IsZero())
		require.False(t, canceled)
	})

	t.Run("done-after-pushing-dependant", func(t *testing.T) {
		one := pushTestJob(t, q, "test", nil, nil)
		two := pushTestJob(t, q, "test", nil, nil)

		j := pushTestJob(t, q, "test", nil, []uuid.UUID{one, two})
		queued, started, finished, canceled, err := q.JobStatus(j, nil)
		require.NoError(t, err)
		require.True(t, !queued.IsZero())
		require.True(t, started.IsZero())
		require.True(t, finished.IsZero())
		require.False(t, canceled)

		r := []uuid.UUID{}
		r = append(r, finishNextTestJob(t, q, "test", testResult{}))
		r = append(r, finishNextTestJob(t, q, "test", testResult{}))
		require.ElementsMatch(t, []uuid.UUID{one, two}, r)

		require.Equal(t, j, finishNextTestJob(t, q, "test", testResult{}))

		queued, started, finished, canceled, err = q.JobStatus(j, &testResult{})
		require.NoError(t, err)
		require.True(t, !queued.IsZero())
		require.True(t, !started.IsZero())
		require.True(t, !finished.IsZero())
		require.False(t, canceled)
	})
}

// Test that a job queue allows parallel access to multiple workers, mainly to
// verify the quirky unlocking in Dequeue().
func TestMultipleWorkers(t *testing.T) {
	q, dir := newTemporaryQueue(t, []string{"octopus", "clownfish"})
	defer cleanupTempDir(t, dir)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		id, err := q.Dequeue(ctx, []string{"octopus"}, &json.RawMessage{})
		require.NoError(t, err)
		require.NotEmpty(t, id)
	}()

	// Increase the likelihood that the above goroutine was scheduled and
	// is waiting in Dequeue().
	time.Sleep(10 * time.Millisecond)

	// This call to Dequeue() should not block on the one in the goroutine.
	id := pushTestJob(t, q, "clownfish", nil, nil)
	r, err := q.Dequeue(context.Background(), []string{"clownfish"}, &json.RawMessage{})
	require.NoError(t, err)
	require.Equal(t, id, r)

	// Now wake up the Dequeue() in the goroutine and wait for it to finish.
	_ = pushTestJob(t, q, "octopus", nil, nil)
	<-done
}

func TestCancel(t *testing.T) {
	q, dir := newTemporaryQueue(t, []string{"octopus", "clownfish"})
	defer cleanupTempDir(t, dir)

	// Cancel a non-existing job
	err := q.CancelJob(uuid.New())
	require.Error(t, err)

	// Cancel a pending job
	id := pushTestJob(t, q, "clownfish", nil, nil)
	require.NotEmpty(t, id)
	err = q.CancelJob(id)
	require.NoError(t, err)
	_, _, _, canceled, err := q.JobStatus(id, &testResult{})
	require.NoError(t, err)
	require.True(t, canceled)
	err = q.FinishJob(id, &testResult{})
	require.Error(t, err)

	// Cancel a running job, which should not dequeue the canceled job from above
	id = pushTestJob(t, q, "clownfish", nil, nil)
	require.NotEmpty(t, id)
	r, err := q.Dequeue(context.Background(), []string{"clownfish"}, &json.RawMessage{})
	require.NoError(t, err)
	require.Equal(t, id, r)
	err = q.CancelJob(id)
	require.NoError(t, err)
	_, _, _, canceled, err = q.JobStatus(id, &testResult{})
	require.NoError(t, err)
	require.True(t, canceled)
	err = q.FinishJob(id, &testResult{})
	require.Error(t, err)

	// Cancel a finished job, which is a no-op
	id = pushTestJob(t, q, "clownfish", nil, nil)
	require.NotEmpty(t, id)
	r, err = q.Dequeue(context.Background(), []string{"clownfish"}, &json.RawMessage{})
	require.NoError(t, err)
	require.Equal(t, id, r)
	err = q.FinishJob(id, &testResult{})
	require.NoError(t, err)
	err = q.CancelJob(id)
	require.NoError(t, err)
	_, _, _, canceled, err = q.JobStatus(id, &testResult{})
	require.NoError(t, err)
	require.False(t, canceled)
}
