package variants

// AssignVariantAgents populates the Agent field for each variant.
// If variantAgents is provided, it cycles through the list (Req 10.3).
// Otherwise, it uses the defaultAgent for all variants.
func AssignVariantAgents(variants []*Variant, variantAgents []string, defaultAgent string) {
	if len(variantAgents) == 0 {
		// No variant-specific agents, use default for all
		for _, v := range variants {
			v.Agent = defaultAgent
		}
		return
	}

	// Assign agents, cycling if fewer agents than variants
	for i, v := range variants {
		v.Agent = variantAgents[i%len(variantAgents)]
	}
}
