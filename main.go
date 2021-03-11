package main

import (
	"context"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/mt-inside/go-openrgb/pkg/model"
	"github.com/mt-inside/go-usvc"
	"github.com/reactivex/rxgo/v2"
)

type gradientTable []struct {
	c    colorful.Color
	time time.Duration
}

const userAgent = "rgbshift"

func mustParseHex(s string) colorful.Color {
	c, err := colorful.Hex(s)
	if err != nil {
		panic("MustParseHex: " + err.Error())
	}
	return c
}
func mustParseTime(s, fmt string) time.Time {
	t, err := time.Parse(s, fmt)
	if err != nil {
		panic("MustParseTime: " + err.Error())
	}
	return t
}
func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic("MustParseDuration: " + err.Error())
	}
	return d
}

func sinceMidnight(t time.Time) time.Duration {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return t.Sub(midnight)
}

func main() {
	log := usvc.GetLogger(false, 0)

	m, err := model.NewModel(log.WithName("go-openrgb"), "localhost:6742", userAgent)
	if err != nil {
		log.Error(err, "Couldn't synchronise devices and colors from server")
		os.Exit(1)
	}

	// FIXME update as we cross midnight local time
	sunrise, noon, sunset, err := getSolarTimes(log)
	if err != nil {
		log.Error(err, "Couldn't get Solar times")
		os.Exit(1)
	}

	wake, sleep := getSchedule()

	keyColors := gradientTable{
		{mustParseHex("#000000"), mustParseDuration("0h")},
		{mustParseHex("#000000"), wake},
		{mustParseHex("#ff0000"), sunrise},
		{mustParseHex("#ffff00"), noon},
		{mustParseHex("#00ffff"), sunset},
		{mustParseHex("#0000ff"), sleep},
		{mustParseHex("#000000"), mustParseDuration("24h")},
	}

	<-rxgo.
		Create([]rxgo.Producer{func(_ context.Context, next chan<- rxgo.Item) {
			timer := time.NewTicker(1 * time.Second)
			for n := range timer.C {
				next <- rxgo.Of(n)
			}
		}}).
		Map(func(_ context.Context, v interface{}) (interface{}, error) {
			n := v.(time.Time)
			d := sinceMidnight(n)
			c := getColor(log, keyColors, d)
			return c, nil
		}).
		DistinctUntilChanged(toHexCtx).
		ForEach(
			func(v interface{}) {
				c := v.(colorful.Color)
				m.SetColor(c)
				err = m.Thither()
				if err != nil {
					log.Error(err, "Couldn't synchronise colors to server")
					os.Exit(1)
				}
				log.Info("Updated color", "color", c.Hex())
			},
			func(err error) {
				panic(err)
			},
			func() {
				log.Info("Fin.")
			},
		)
}

func toHexCtx(_ context.Context, x interface{}) (interface{}, error) {
	c := x.(colorful.Color)
	return c.Hex(), nil
}

func getSchedule() (sunrise, sunset time.Duration) {
	return mustParseDuration("7h"), mustParseDuration("23h")
}

// Heavily inspired by: https://github.com/lucasb-eyer/go-colorful/blob/master/doc/gradientgen/gradientgen.go
func getColor(log logr.Logger, gt gradientTable, t time.Duration) colorful.Color {
	// before the first keycolor
	if t < gt[0].time {
		log.V(2).Info("Next key color", "time", gt[0].time, "color", gt[0].c)
		return gt[0].c
	}

	// in the range
	for i := 0; i < len(gt)-1; i++ {
		c1 := gt[i]
		c2 := gt[i+1]
		if c1.time <= t && t <= c2.time {
			t := float64(t-c1.time) / float64(c2.time-c1.time)
			log.V(2).Info("Previous key color", "time", c1.time, "color", c1.c)
			log.V(2).Info("Next key color", "time", c2.time, "color", c2.c)
			log.V(1).Info("Interpolating color", "fraction", t)
			return c1.c.BlendLab(c2.c, t).Clamped() //super important to blend in a decent colorspace
		}
	}

	// after the last keycolor
	log.V(2).Info("Previous key color", "time", gt[len(gt)-1].time, "color", gt[len(gt)-1].c)
	return gt[len(gt)-1].c
}
