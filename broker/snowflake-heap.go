/*
Keeping track of pending available snowflake proxies.
*/

package main

/*
The Snowflake struct contains a single interaction
over the offer and answer channels.
*/
type Snowflake struct {
	id            string
	proxyType     string
	natType       string
	offerChannel  chan *ClientOffer
	answerChannel chan string
	clients       int
	index         int
}

// Implements heap.Interface, and holds Snowflakes.
type SnowflakeHeap []*Snowflake

func (sh SnowflakeHeap) Len() int { return len(sh) }

func (sh SnowflakeHeap) Less(i, j int) bool {
	// Snowflakes serving less clients should sort earlier.
	return sh[i].clients < sh[j].clients
}

func (sh SnowflakeHeap) Swap(i, j int) {
	sh[i], sh[j] = sh[j], sh[i]
	sh[i].index = i
	sh[j].index = j
}

func (sh *SnowflakeHeap) Push(s interface{}) {
	n := len(*sh)
	snowflake := s.(*Snowflake)
	snowflake.index = n
	*sh = append(*sh, snowflake)
}

// Only valid when Len() > 0.
func (sh *SnowflakeHeap) Pop() interface{} {
	flakes := *sh
	n := len(flakes)
	snowflake := flakes[n-1]
	snowflake.index = -1
	*sh = flakes[0 : n-1]
	return snowflake
}
