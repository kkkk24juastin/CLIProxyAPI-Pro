package management

// Shutdown stops the remaining host-owned management background controllers.
// Account inspection lifecycle is owned by pro-observability.
func (h *Handler) Shutdown() {
	if h == nil {
		return
	}
	h.shutdownOnce.Do(func() {
		if h.lifecycleCancel != nil {
			h.lifecycleCancel()
		}
		h.lifecycleWG.Wait()
		stopRoutingPolicyController(h)
	})
}
