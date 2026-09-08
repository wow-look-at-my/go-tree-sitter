package treesitter

func rangeArrayAdd(self *[]Range, start, end length) {
	if len(*self) > 0 {
		last := &(*self)[len(*self)-1]
		if start.Bytes <= last.EndByte {
			last.EndByte = end.Bytes
			last.EndPoint = end.Extent
			return
		}
	}
	if start.Bytes < end.Bytes {
		*self = append(*self, Range{
			StartPoint: start.Extent,
			EndPoint:   end.Extent,
			StartByte:  start.Bytes,
			EndByte:    end.Bytes,
		})
	}
}

func rangeArrayIntersects(self []Range, startIndex uint32, startByte, endByte uint32) bool {
	for i := startIndex; i < uint32(len(self)); i++ {
		r := self[i]
		if r.EndByte > startByte {
			if r.StartByte >= endByte {
				break
			}
			return true
		}
	}
	return false
}

func rangeArrayGetChangedRanges(oldRanges, newRanges []Range, differences *[]Range) {
	newIndex := 0
	oldIndex := 0
	currentPosition := lengthZero()
	inOldRange := false
	inNewRange := false

	for oldIndex < len(oldRanges) || newIndex < len(newRanges) {
		var nextOldPosition length
		switch {
		case inOldRange:
			nextOldPosition = length{Bytes: oldRanges[oldIndex].EndByte, Extent: oldRanges[oldIndex].EndPoint}
		case oldIndex < len(oldRanges):
			nextOldPosition = length{Bytes: oldRanges[oldIndex].StartByte, Extent: oldRanges[oldIndex].StartPoint}
		default:
			nextOldPosition = lengthMax
		}

		var nextNewPosition length
		switch {
		case inNewRange:
			nextNewPosition = length{Bytes: newRanges[newIndex].EndByte, Extent: newRanges[newIndex].EndPoint}
		case newIndex < len(newRanges):
			nextNewPosition = length{Bytes: newRanges[newIndex].StartByte, Extent: newRanges[newIndex].StartPoint}
		default:
			nextNewPosition = lengthMax
		}

		switch {
		case nextOldPosition.Bytes < nextNewPosition.Bytes:
			if inOldRange != inNewRange {
				rangeArrayAdd(differences, currentPosition, nextOldPosition)
			}
			if inOldRange {
				oldIndex++
			}
			currentPosition = nextOldPosition
			inOldRange = !inOldRange
		case nextNewPosition.Bytes < nextOldPosition.Bytes:
			if inOldRange != inNewRange {
				rangeArrayAdd(differences, currentPosition, nextNewPosition)
			}
			if inNewRange {
				newIndex++
			}
			currentPosition = nextNewPosition
			inNewRange = !inNewRange
		default:
			if inOldRange != inNewRange {
				rangeArrayAdd(differences, currentPosition, nextNewPosition)
			}
			if inOldRange {
				oldIndex++
			}
			if inNewRange {
				newIndex++
			}
			inOldRange = !inOldRange
			inNewRange = !inNewRange
			currentPosition = nextNewPosition
		}
	}
}
