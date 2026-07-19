package manifest

type ChangeType string

const (
	Unchanged        ChangeType = "unchanged"
	LocalOnlyChange  ChangeType = "localOnlyChange"
	RemoteOnlyChange ChangeType = "remoteOnlyChange"
	BothChanged      ChangeType = "bothChanged"
	LocalDelete      ChangeType = "localDelete"
	RemoteDelete     ChangeType = "remoteDelete"
)

func Diff(local, base, remote Manifest) map[string]ChangeType {
	localMap := local.Map()
	baseMap := base.Map()
	remoteMap := remote.Map()

	paths := make(map[string]struct{})
	for path := range localMap {
		paths[path] = struct{}{}
	}
	for path := range baseMap {
		paths[path] = struct{}{}
	}
	for path := range remoteMap {
		paths[path] = struct{}{}
	}

	result := make(map[string]ChangeType, len(paths))
	for path := range paths {
		localEntry, inLocal := localMap[path]
		baseEntry, inBase := baseMap[path]
		remoteEntry, inRemote := remoteMap[path]

		localChanged := changed(inLocal, inBase, localEntry.SHA256, baseEntry.SHA256)
		remoteChanged := changed(inRemote, inBase, remoteEntry.SHA256, baseEntry.SHA256)

		result[path] = classify(localChanged, remoteChanged, inLocal, inRemote, inBase)
	}

	return result
}

func changed(present, basePresent bool, hash, baseHash string) bool {
	if present != basePresent {
		return true
	}
	if present && basePresent && hash != baseHash {
		return true
	}
	return false
}

func classify(localChanged, remoteChanged, inLocal, inRemote, inBase bool) ChangeType {
	switch {
	case localChanged && remoteChanged:
		return BothChanged
	case localChanged:
		if !inLocal && inBase {
			return LocalDelete
		}
		return LocalOnlyChange
	case remoteChanged:
		if !inRemote && inBase {
			return RemoteDelete
		}
		return RemoteOnlyChange
	default:
		return Unchanged
	}
}
