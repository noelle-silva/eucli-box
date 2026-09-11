import { describe, expect, it } from 'vitest'
import { isReleaseSnapshotFreshForKind, RELEASE_SNAPSHOT_FRESHNESS_MS, type ReleaseCheckSnapshot } from './release'

function snapshotWithCheckedAt(source: string, checkedAt: string): ReleaseCheckSnapshot {
  return {
    status: 'completed',
    source,
    startedAt: checkedAt,
    checkedAt,
    results: [
      {
        artifact: { kind: 'tool', id: 'context7' },
        source: { kind: 'tool', repository: '', owner: '', name: '' },
        installed: false,
        currentVersion: '',
        latestVersion: '0.1.0',
        status: 'completed',
        checkedAt,
        updateAvailable: true,
        releaseUrl: '',
        releaseNotes: '',
        downloadSize: 0,
        compatibility: null,
        affectedArtifacts: [],
        failureReason: '',
      },
    ],
    failureReason: '',
  }
}

describe('isReleaseSnapshotFreshForKind', () => {
  it('accepts a matching source with a recent checked time', () => {
    const snapshot = snapshotWithCheckedAt('official', new Date(Date.now() - 60_000).toISOString())
    expect(isReleaseSnapshotFreshForKind(snapshot, 'official', 'tool')).toBe(true)
  })

  it('rejects a snapshot from another source', () => {
    const snapshot = snapshotWithCheckedAt('local', new Date().toISOString())
    expect(isReleaseSnapshotFreshForKind(snapshot, 'official', 'tool')).toBe(false)
  })

  it('rejects an expired snapshot', () => {
    const expired = new Date(Date.now() - RELEASE_SNAPSHOT_FRESHNESS_MS - 60_000).toISOString()
    const snapshot = snapshotWithCheckedAt('official', expired)
    expect(isReleaseSnapshotFreshForKind(snapshot, 'official', 'tool')).toBe(false)
  })

  it('rejects a snapshot without results for the requested artifact kind', () => {
    const snapshot = snapshotWithCheckedAt('official', new Date().toISOString())
    expect(isReleaseSnapshotFreshForKind(snapshot, 'official', 'plugin')).toBe(false)
  })

  it('rejects an absent snapshot or an invalid checked time', () => {
    expect(isReleaseSnapshotFreshForKind(null, 'official', 'tool')).toBe(false)
    expect(isReleaseSnapshotFreshForKind(snapshotWithCheckedAt('official', ''), 'official', 'tool')).toBe(false)
  })
})
