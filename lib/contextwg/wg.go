package contextwg

import (
	"context"
	"errors"
	"log"
	"sync"
)

type wgKeyType string

const KEY = wgKeyType("contextwg") //use unexported type to prevent key collisions

var NoWgFound = errors.New("no WaitGroup found")

func WithWaitGroup(parent context.Context, wg *sync.WaitGroup) context.Context {
	return context.WithValue(parent, KEY, wg)
}

func Get(ctx context.Context) (wg *sync.WaitGroup, err error) {
	value := ctx.Value(KEY)
	if value == nil {
		return nil, NoWgFound
	}
	wg, ok := value.(*sync.WaitGroup)
	if !ok {
		return nil, errors.New("wg is not of type *sync.WaitGroup")
	}
	return wg, nil
}

func AddWithErr(ctx context.Context, delta int) (err error) {
	wg, err := Get(ctx)
	if err != nil {
		return err
	}
	wg.Add(delta)
	return nil
}

func Add(ctx context.Context, delta int) {
	err := AddWithErr(ctx, delta)
	if err == NoWgFound {
		return
	}
	if err != nil {
		log.Println("WARNING:", err)
	}
}

func DoneWithErr(ctx context.Context) (err error) {
	wg, err := Get(ctx)
	if err != nil {
		return err
	}
	wg.Done()
	return nil
}

func Done(ctx context.Context) {
	err := DoneWithErr(ctx)
	if err == NoWgFound {
		return
	}
	if err != nil {
		log.Println("WARNING:", err)
	}
}

func WaitWithErr(ctx context.Context) (err error) {
	wg, err := Get(ctx)
	if err != nil {
		return err
	}
	wg.Wait()
	return nil
}

func Wait(ctx context.Context) {
	err := WaitWithErr(ctx)
	if err == NoWgFound {
		return
	}
	if err != nil {
		log.Println("WARNING:", err)
	}
}
