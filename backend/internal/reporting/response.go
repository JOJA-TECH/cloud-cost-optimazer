package reporting

import "github.com/google/uuid"

type ReportResponse struct {
	Report   Report         `json:"report"`
	Artifact ReportArtifact `json:"artifact"`
}

func newReportResponse(report Report) (ReportResponse, error) {
	if report.Artifact == nil || report.Artifact.UID == uuid.Nil {
		return ReportResponse{}, ErrReportGenerationFailed
	}
	artifact := *report.Artifact
	report.Artifact = nil
	return ReportResponse{Report: report, Artifact: artifact}, nil
}
