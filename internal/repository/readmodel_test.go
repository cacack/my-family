package repository

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestPage(t *testing.T) {
	items := []int{0, 1, 2, 3, 4}
	tests := []struct {
		name          string
		offset, limit int
		want          []int
	}{
		{"first page", 0, 2, []int{0, 1}},
		{"last partial page", 4, 2, []int{4}},
		{"past the end", 9, 2, []int{}},
		{"negative offset is zero", -3, 2, []int{0, 1}},
		{"zero limit is empty", 1, 0, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Page(items, tt.offset, tt.limit); !slices.Equal(got, tt.want) {
				t.Errorf("Page(%d, %d) = %v, want %v", tt.offset, tt.limit, got, tt.want)
			}
		})
	}
}

func TestListAll(t *testing.T) {
	t.Run("zero records", func(t *testing.T) {
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			return nil, 0, nil
		}

		result, err := ListAll(context.Background(), 10, listFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected 0 records, got %d", len(result))
		}
	})

	t.Run("fewer than page size", func(t *testing.T) {
		data := []int{1, 2, 3}
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			end := opts.Offset + opts.Limit
			if end > len(data) {
				end = len(data)
			}
			start := opts.Offset
			if start > len(data) {
				start = len(data)
			}
			return data[start:end], len(data), nil
		}

		result, err := ListAll(context.Background(), 10, listFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("expected 3 records, got %d", len(result))
		}
		for i, v := range result {
			if v != data[i] {
				t.Errorf("result[%d] = %d, want %d", i, v, data[i])
			}
		}
	})

	t.Run("exactly page size boundary", func(t *testing.T) {
		data := []int{1, 2, 3, 4, 5}
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			end := opts.Offset + opts.Limit
			if end > len(data) {
				end = len(data)
			}
			start := opts.Offset
			if start > len(data) {
				start = len(data)
			}
			return data[start:end], len(data), nil
		}

		result, err := ListAll(context.Background(), 5, listFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 5 {
			t.Errorf("expected 5 records, got %d", len(result))
		}
	})

	t.Run("multiple pages", func(t *testing.T) {
		data := make([]int, 25)
		for i := range data {
			data[i] = i + 1
		}
		callCount := 0
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			callCount++
			end := opts.Offset + opts.Limit
			if end > len(data) {
				end = len(data)
			}
			start := opts.Offset
			if start > len(data) {
				start = len(data)
			}
			return data[start:end], len(data), nil
		}

		result, err := ListAll(context.Background(), 10, listFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 25 {
			t.Errorf("expected 25 records, got %d", len(result))
		}
		if callCount != 3 {
			t.Errorf("expected 3 calls (pages of 10+10+5), got %d", callCount)
		}
		for i, v := range result {
			if v != i+1 {
				t.Errorf("result[%d] = %d, want %d", i, v, i+1)
			}
		}
	})

	t.Run("error handling", func(t *testing.T) {
		expectedErr := fmt.Errorf("database error")
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			return nil, 0, expectedErr
		}

		result, err := ListAll(context.Background(), 10, listFn)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
		if result != nil {
			t.Errorf("expected nil result, got %v", result)
		}
	})

	t.Run("error on second page", func(t *testing.T) {
		expectedErr := fmt.Errorf("page 2 error")
		callCount := 0
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			callCount++
			if callCount == 2 {
				return nil, 0, expectedErr
			}
			return []int{1, 2, 3}, 10, nil
		}

		result, err := ListAll(context.Background(), 3, listFn)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
		if result != nil {
			t.Errorf("expected nil result on error, got %v", result)
		}
	})

	t.Run("pre-allocates capacity from total", func(t *testing.T) {
		data := []int{1, 2, 3}
		listFn := func(_ context.Context, opts ListOptions) ([]int, int, error) {
			end := opts.Offset + opts.Limit
			if end > len(data) {
				end = len(data)
			}
			return data[opts.Offset:end], len(data), nil
		}

		result, err := ListAll(context.Background(), 2, listFn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("expected 3 records, got %d", len(result))
		}
		if cap(result) < 3 {
			t.Errorf("expected capacity >= 3, got %d", cap(result))
		}
	})
}
