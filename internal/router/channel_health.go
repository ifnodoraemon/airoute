package router

// ChannelHealthCounts reads configured channels without taking canary slots.
func (d *Dispatcher) ChannelHealthCounts() (tripped, disabled int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, ch := range d.channels {
		if ch.Status != "active" {
			disabled++
		}
		switch d.GetBreakerStatus(ch.Name) {
		case "OPEN", "HALF-OPEN":
			tripped++
		}
	}
	return
}
