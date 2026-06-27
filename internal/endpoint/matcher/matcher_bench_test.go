package matcher

import (
	"fmt"
	"testing"
)

func BenchmarkPrefixMatcher(b *testing.B) {
	for _, count := range []int{4, 16, 64, 256, 1024} {
		b.Run(fmt.Sprintf("patterns_%d_miss", count), func(b *testing.B) {
			m := NewPrefix(benchPrefixes(count))
			value := "/var/log/application/current/request.log"
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = m.Match(value)
			}
		})
		b.Run(fmt.Sprintf("patterns_%d_hit", count), func(b *testing.B) {
			m := NewPrefix(benchPrefixes(count))
			value := fmt.Sprintf("/bench/path/%04d/payload", count-1)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = m.Match(value)
			}
		})
	}
}

func BenchmarkExactMatcher(b *testing.B) {
	for _, count := range []int{4, 16, 64, 256, 1024} {
		b.Run(fmt.Sprintf("patterns_%d", count), func(b *testing.B) {
			m := NewExact(benchPorts(count))
			value := fmt.Sprintf("%d", 10000+count-1)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = m.Match(value)
			}
		})
	}
}

func BenchmarkContainsMatcher(b *testing.B) {
	for _, count := range []int{4, 16, 64, 256, 1024} {
		b.Run(fmt.Sprintf("patterns_%d_miss", count), func(b *testing.B) {
			m := NewContains(benchTokens(count))
			value := "/usr/bin/python app.py --serve"
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = m.Match(value)
			}
		})
		b.Run(fmt.Sprintf("patterns_%d_hit", count), func(b *testing.B) {
			m := NewContains(benchTokens(count))
			value := fmt.Sprintf("/bin/sh -c token-%04d", count-1)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = m.Match(value)
			}
		})
	}
}

func benchPrefixes(count int) []string {
	values := make([]string, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, fmt.Sprintf("/bench/path/%04d/", i))
	}
	return values
}

func benchPorts(count int) []string {
	values := make([]string, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, fmt.Sprintf("%d", 10000+i))
	}
	return values
}

func benchTokens(count int) []string {
	values := make([]string, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, fmt.Sprintf("token-%04d", i))
	}
	return values
}
