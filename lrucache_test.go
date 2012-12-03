package lrucache

import (
	"strconv"
	"testing"
)

type flatsize int

func (i flatsize) Size() int64 {
	return 1
}

type varsize int

func (i varsize) Size() int64 {
	return int64(i)
}

type purgeable struct {
	purged bool
	how    bool
}

func (x *purgeable) Size() int64 {
	return 1
}

func (x *purgeable) OnPurge(d bool) {
	x.purged = true
	x.how = d
}

// Wait until a cache has processed all its operations
func sync(c *Cache) {
	ch := make(chan bool)
	c.opChan <- reqPing(ch)
	<-ch
}

func TestOnPurge_1(t *testing.T) {
	c := New(1)
	var x, y purgeable
	c.Set("x", &x)
	c.Set("y", &y)
	sync(c)
	if !x.purged {
		t.Error("Element was not purged from full cache")
	}
	if x.how {
		t.Error("Element should have been purged but was deleted")
	}
}

func TestOnPurge_2(t *testing.T) {
	c := New(1)
	var x purgeable
	c.Set("x", &x)
	c.Delete("x")
	sync(c)
	if !x.purged {
		t.Error("Element was not deleted from cache")
	}
	if !x.how {
		t.Error("Element should have been deleted but was purged")
	}
}

// Just test filling a cache with a type that does not implement NotifyPurge
func TestsafeOnPurge(t *testing.T) {
	c := New(1)
	i := varsize(1)
	j := varsize(1)
	c.Set("i", i)
	c.Set("j", j)
	sync(c)
}

func TestSize(t *testing.T) {
	c := New(100)
	// sum(0..14) = 105
	for i := 1; i < 15; i++ {
		c.Set(strconv.Itoa(i), varsize(i))
	}
	sync(c)
	// At this point, expect {0, 1, 2, 3} to be purged
	if c.Size() != 99 {
		t.Errorf("Unexpected size: %d", c.Size())
	}
	for i := 0; i < 4; i++ {
		if _, ok := c.Get(strconv.Itoa(i)); ok {
			t.Errorf("Expected %d to be purged", i)
		}
	}
	for i := 4; i < 15; i++ {
		if _, ok := c.Get(strconv.Itoa(i)); !ok {
			t.Errorf("Expected %d to be cached", i)
		}
	}
}

func TestOnMiss(t *testing.T) {
	c := New(10)
	// Expected cache misses (arbitrary value)
	misses := map[string]int{}
	for i := 5; i < 10; i++ {
		misses[strconv.Itoa(i)] = 0
	}
	c.OnMiss(func(id string) Cacheable {
		if _, ok := misses[id]; !ok {
			t.Errorf("Unexpected cache miss for %q", id)
		} else {
			delete(misses, id)
		}
		i, err := strconv.Atoi(id)
		if err != nil {
			t.Fatalf("Illegal id: %q", id)
		}
		return flatsize(flatsize(i))
	})
	for i := 0; i < 5; i++ {
		c.Set(strconv.Itoa(i), flatsize(i))
	}
	for i := 0; i < 10; i++ {
		x, ok := c.Get(strconv.Itoa(i))
		if !ok {
			t.Errorf("Unexpected not-found Get for %d", i)
			continue
		}
		if j := x.(flatsize); int(j) != i {
			t.Errorf("Illegal cache value: expected %d, got %d", i, j)
		}
	}
	for k := range misses {
		t.Errorf("Expected %s to miss", k)
	}
}

func TestConcurrentOnMiss(t *testing.T) {
	c := New(10)
	ch := make(chan flatsize)
	// If key foo is requested but not cached, read it from the channel
	c.OnMiss(func(id string) Cacheable {
		if id == "foo" {
			// Indicate that we want a value
			ch <- flatsize(0)
			return <-ch
		}
		return nil
	})
	go func() {
		c.Get("foo")
	}()
	<-ch
	// Now we know for sure: a goroutine is blocking on c.Get("foo").
	// But other cache operations should be unaffected:
	c.Set("bar", flatsize(10))
	// Unlock that poor blocked goroutine
	ch <- flatsize(10)
	if result, ok := c.Get("foo"); !ok || result != flatsize(10) {
		t.Errorf("Expected 10, got: %d", result)
	}
}
