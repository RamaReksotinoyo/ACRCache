package main

// lruNode is a node in a doubly linked list.
type lruNode struct {
	key   string
	value any
	prev  *lruNode
	next  *lruNode
}

// lruList is a doubly linked list that acts as an LRU list.
// The front (head.next) is the most recently used entry.
// The back (tail.prev) is the least recently used entry.
type lruList struct {
	head  *lruNode // sentinel head
	tail  *lruNode // sentinel tail
	items map[string]*lruNode
	size  int
}

// newLRUList creates an empty LRU list.
func newLRUList() *lruList {
	head := &lruNode{}
	tail := &lruNode{}
	head.next = tail
	tail.prev = head
	return &lruList{
		head:  head,
		tail:  tail,
		items: make(map[string]*lruNode),
	}
}

// has returns true if the key exists in the list.
func (l *lruList) has(key string) bool {
	_, ok := l.items[key]
	return ok
}

// get returns the node for the given key, or nil if not found.
func (l *lruList) get(key string) *lruNode {
	return l.items[key]
}

// pushFront adds a new node at the front (MRU position).
func (l *lruList) pushFront(key string, value any) *lruNode {
	node := &lruNode{key: key, value: value}
	l.insertFront(node)
	l.items[key] = node
	l.size++
	return node
}

// moveToFront moves an existing node to the front (MRU position).
func (l *lruList) moveToFront(node *lruNode) {
	l.remove(node)
	l.insertFront(node)
}

// removeLRU removes and returns the least recently used node (back of list).
func (l *lruList) removeLRU() *lruNode {
	if l.size == 0 {
		return nil
	}
	node := l.tail.prev
	l.remove(node)
	delete(l.items, node.key)
	l.size--
	return node
}

// removeKey removes a node by key.
func (l *lruList) removeKey(key string) *lruNode {
	node, ok := l.items[key]
	if !ok {
		return nil
	}
	l.remove(node)
	delete(l.items, key)
	l.size--
	return node
}

// insertFront inserts a node right after the sentinel head.
func (l *lruList) insertFront(node *lruNode) {
	node.prev = l.head
	node.next = l.head.next
	l.head.next.prev = node
	l.head.next = node
}

// remove unlinks a node from the list (does not update items map or size).
func (l *lruList) remove(node *lruNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

// keys returns all keys in the list from MRU to LRU order.
func (l *lruList) keys() []string {
	result := make([]string, 0, l.size)
	cur := l.head.next
	for cur != l.tail {
		result = append(result, cur.key)
		cur = cur.next
	}
	return result
}
