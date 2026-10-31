package isureal

import (
	"container/list"
	"sync"
	"time"
)

// 画像の中身と持ち主の鍵は変わらないので、台ごとにずっと持つ。DB が別リージョンにある台では、取得のたびに 100ms の往復と画像の転送がかかるため。
// 持ち主の状態と参照（削除されたか）は変わるので、短い間だけ持つ。削除の反映には 5 秒の猶予がある（manual.md「画像URLの検証」）。
const imageStateTTL = 2 * time.Second
const imageDataCacheBytes = 512 << 20

type imageState struct {
	key         []byte
	mediaType   string
	ownerActive bool
	referenced  bool
	checkedAt   time.Time
}

type imageDataEntry struct {
	id   string
	data []byte
}

type ImageCache struct {
	mu        sync.Mutex
	states    map[string]imageState
	data      map[string]*list.Element
	order     *list.List
	dataBytes int
}

func newImageCache() *ImageCache {
	return &ImageCache{states: make(map[string]imageState), data: make(map[string]*list.Element), order: list.New()}
}

func (c *ImageCache) state(id string) (imageState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.states[id]
	if !ok || time.Since(state.checkedAt) >= imageStateTTL {
		return imageState{}, false
	}
	return state, true
}

func (c *ImageCache) setState(id string, state imageState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.states) >= 100000 {
		for k, s := range c.states {
			if time.Since(s.checkedAt) >= imageStateTTL {
				delete(c.states, k)
			}
		}
	}
	c.states[id] = state
}

// この台で画像を無効にする書き込みをしたら、ほかの画像の分も含めて状態を捨てる（書き込みは少なく、捨てても 1 往復で戻る）
func (c *ImageCache) ForgetStates() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states = make(map[string]imageState)
}

func (c *ImageCache) getData(id string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.data[id]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*imageDataEntry).data, true
}

func (c *ImageCache) setData(id string, data []byte) {
	if len(data) > imageDataCacheBytes/16 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[id]; ok {
		return
	}
	c.data[id] = c.order.PushFront(&imageDataEntry{id: id, data: data})
	c.dataBytes += len(data)
	for c.dataBytes > imageDataCacheBytes {
		oldest := c.order.Back()
		entry := oldest.Value.(*imageDataEntry)
		c.order.Remove(oldest)
		delete(c.data, entry.id)
		c.dataBytes -= len(entry.data)
	}
}
