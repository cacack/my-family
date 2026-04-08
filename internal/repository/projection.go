	// 8. Transfer attributes from merged person to survivor
	attributes, err := p.readStore.ListAttributesForPerson(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, attr := range attributes {
		attr.PersonID = e.SurvivorID
		if err := p.readStore.SaveAttribute(ctx, &attr); err != nil {
			return err
		}
	}

	// 8.1 Transfer evidence analyses from merged person to survivor
	analysesList, _, err := p.readStore.ListEvidenceAnalyses(ctx, ListOptions{Limit: 10000})
	if err != nil {
		return err
	}
	for _, analysis := range analysesList {
		if analysis.SubjectID == e.MergedID {
			analysis.SubjectID = e.SurvivorID
			analysis.UpdatedAt = e.OccurredAt()
			if err := p.readStore.SaveEvidenceAnalysis(ctx, &analysis); err != nil {
				return err
			}
		}
	}

	// 8.2 Transfer evidence conflicts from merged person to survivor
	conflicts, err := p.readStore.GetConflictsForSubject(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, conflict := range conflicts {
		conflict.SubjectID = e.SurvivorID
		conflict.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SaveEvidenceConflict(ctx, &conflict); err != nil {
			return err
		}
	}

	// 8.3 Transfer research logs from merged person to survivor
	logs, err := p.readStore.GetResearchLogsForSubject(ctx, e.MergedID)
	if err != nil {
		return err
	}
	for _, log := range logs {
		log.SubjectID = e.SurvivorID
		log.UpdatedAt = e.OccurredAt()
		if err := p.readStore.SaveResearchLog(ctx, &log); err != nil {
			return err
		}
	}

	// 8.4 Transfer proof summaries from merged person to survivor
	summariesList, _, err := p.readStore.ListProofSummaries(ctx, ListOptions{Limit: 10000})
	if err != nil {
		return err
	}
	for _, summary := range summariesList {
		if summary.SubjectID == e.MergedID {
			summary.SubjectID = e.SurvivorID
			summary.UpdatedAt = e.OccurredAt()
			if err := p.readStore.SaveProofSummary(ctx, &summary); err != nil {
				return err
			}
		}
	}

	// 9. Delete merged person from read model