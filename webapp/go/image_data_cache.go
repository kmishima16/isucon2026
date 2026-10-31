package isureal

import (
	"container/list"
	"sync"
)

// 画像の中身は id ごとに変わらない（id は UUID で使い回さない）ので、台ごとのメモリに持つ。
// 取得のたびに DB から中身（新しい投稿で平均約 360KB）を読むと、DB の台の CPU と、DB から遠い台の帯域を使っていた（#8）。
// 持ち主や投稿が生きているかは持たず、毎回 DB に聞く。なので、ほかの台での削除や初期化を知らなくても古い画像は返らない
const imageDataCacheBytes = 512 << 20

type imageDataEntry struct {
	id   string
	data []byte
}

type ImageDataCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order *list.List
	bytes int
}

func newImageDataCache() *ImageDataCache {
	return &ImageDataCache{items: make(map[string]*list.Element), order: list.New()}
}

func (c *ImageDataCache) get(id string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[id]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*imageDataEntry).data, true
}

func (c *ImageDataCache) set(id string, data []byte) {
	if len(data) > imageDataCacheBytes/16 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[id]; ok {
		return
	}
	c.items[id] = c.order.PushFront(&imageDataEntry{id: id, data: data})
	c.bytes += len(data)
	for c.bytes > imageDataCacheBytes {
		oldest := c.order.Back()
		entry := oldest.Value.(*imageDataEntry)
		c.order.Remove(oldest)
		delete(c.items, entry.id)
		c.bytes -= len(entry.data)
	}
}
