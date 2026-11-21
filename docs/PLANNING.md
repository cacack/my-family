# Planning

How work is grouped, ordered and scheduled. Card content follows the issue standard (see
[Answer the Acceptance Criteria](../CONTRIBUTING.md#5-answer-the-acceptance-criteria)).

## Milestones, Rank and Sprints

- **Milestone**: what. A themed body of work whose description carries a checkable closure
  condition. Every open issue belongs to one. **Backlog** holds only untriaged issues; ungroomed
  future features go in **Future Candidates**.
- **Rank**: order. The single-select `Rank` field on the
  [my-family Roadmap project](https://github.com/users/cacack/projects/3) ranks milestones
  (`01 …`, `02 …`). `v1.0 - Production Ready` is a release gate, not a step in the order. New
  issues are not added to the project automatically: add them and set their milestone's Rank.
- **Sprint**: when. The `Sprint` iteration field on the same project. Sprints run two weeks,
  Monday to Sunday. Sprint 1 was 2026-11-21 to 2026-12-06; Sprint 2 starts 2026-12-07.

## Cadence

**Planning: the Sunday before each sprint.** A scheduled routine opens a `Sprint N planning
brief` issue (label `planning`) that morning. Planning then:

1. Triages Backlog: each issue gets a milestone and Rank, or is closed.
2. Answers the owner questions on the issues about to be picked. An agent cannot start a card that
   is waiting on a decision.
3. Moves unfinished issues to the next sprint, and commits the next slice from the top of Rank,
   active in at most two or three milestones at once. Finishing milestones is what shrinks the
   list.
4. Plans by issue count: the number closed in recent sprints, not points.
5. Runs `delivery:milestone-review` on any milestone that closed, then closes the brief.

The brief issue is the one kind of open issue with no milestone.

**Direction review: every third sprint, before planning.** Engineering (`panels:panel-engineering`)
and product (`panels:panel-product`) reviews alternate, starting with engineering on 2027-01-03.
The product review reads a `CONSTITUTION.md`, which does not exist yet; derive it from
[ETHOS.md](./ETHOS.md) with `panels:constitution` before the first product review (2027-02-14).
Findings go to Backlog, never straight into a running sprint; planning triages them like anything
else. A review that produces more than about ten issues is itself a finding: the milestones are
pointed the wrong way, or the review needs a tighter scope.
