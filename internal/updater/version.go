package updater

import (
	"strconv"
	"strings"
)

type semanticVersion struct {
	numbers    []uint64
	prerelease []string
	valid      bool
}

// IsNewerVersion compares semantic release tags, including prerelease ordering.
func IsNewerVersion(current, latest string) bool {
	currentVersion := parseSemanticVersion(current)
	latestVersion := parseSemanticVersion(latest)
	if !latestVersion.valid {
		return false
	}
	if !currentVersion.valid {
		return strings.TrimSpace(current) != strings.TrimSpace(latest)
	}
	return compareSemanticVersions(latestVersion, currentVersion) > 0
}

func parseSemanticVersion(value string) semanticVersion {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "v"))
	if buildIndex := strings.IndexByte(value, '+'); buildIndex >= 0 {
		value = value[:buildIndex]
	}
	mainPart := value
	prePart := ""
	if preIndex := strings.IndexByte(value, '-'); preIndex >= 0 {
		mainPart = value[:preIndex]
		prePart = value[preIndex+1:]
		if prePart == "" {
			return semanticVersion{}
		}
	}
	segments := strings.Split(mainPart, ".")
	if len(segments) == 0 {
		return semanticVersion{}
	}
	numbers := make([]uint64, len(segments))
	for index, segment := range segments {
		if segment == "" || (len(segment) > 1 && segment[0] == '0') {
			return semanticVersion{}
		}
		number, err := strconv.ParseUint(segment, 10, 64)
		if err != nil {
			return semanticVersion{}
		}
		numbers[index] = number
	}
	var prerelease []string
	if prePart != "" {
		prerelease = strings.Split(prePart, ".")
		for _, identifier := range prerelease {
			if identifier == "" {
				return semanticVersion{}
			}
		}
	}
	return semanticVersion{numbers: numbers, prerelease: prerelease, valid: true}
}

func compareSemanticVersions(left, right semanticVersion) int {
	length := max(len(left.numbers), len(right.numbers))
	for index := 0; index < length; index++ {
		var leftNumber, rightNumber uint64
		if index < len(left.numbers) {
			leftNumber = left.numbers[index]
		}
		if index < len(right.numbers) {
			rightNumber = right.numbers[index]
		}
		if leftNumber > rightNumber {
			return 1
		}
		if leftNumber < rightNumber {
			return -1
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) > 0 {
		return 1
	}
	if len(left.prerelease) > 0 && len(right.prerelease) == 0 {
		return -1
	}
	for index := 0; index < max(len(left.prerelease), len(right.prerelease)); index++ {
		if index >= len(left.prerelease) {
			return -1
		}
		if index >= len(right.prerelease) {
			return 1
		}
		leftID, rightID := left.prerelease[index], right.prerelease[index]
		leftNumber, leftErr := strconv.ParseUint(leftID, 10, 64)
		rightNumber, rightErr := strconv.ParseUint(rightID, 10, 64)
		switch {
		case leftErr == nil && rightErr == nil && leftNumber > rightNumber:
			return 1
		case leftErr == nil && rightErr == nil && leftNumber < rightNumber:
			return -1
		case leftErr == nil && rightErr != nil:
			return -1
		case leftErr != nil && rightErr == nil:
			return 1
		case leftID > rightID:
			return 1
		case leftID < rightID:
			return -1
		}
	}
	return 0
}
