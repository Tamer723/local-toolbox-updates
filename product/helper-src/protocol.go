package main

const ProtocolVersion = 1

type NativeCommand string

const (
	CommandPing                 NativeCommand = "ping"
	CommandGetSettings          NativeCommand = "get_settings"
	CommandSaveSettings         NativeCommand = "save_settings"
	CommandCheckTools           NativeCommand = "check_tools"
	CommandPickFile             NativeCommand = "pick_file"
	CommandPickOutputFolder     NativeCommand = "pick_output_folder"
	CommandOpenPath             NativeCommand = "open_path"
	CommandFetchInfo            NativeCommand = "fetch_info"
	CommandConvertMP3           NativeCommand = "convert_mp3"
	CommandDownloadVideo        NativeCommand = "download_video"
	CommandDownloadAudio        NativeCommand = "download_audio"
	CommandDownloadThumbnail    NativeCommand = "download_thumbnail"
	CommandDownloadSubtitles    NativeCommand = "download_subtitles"
	CommandDownloadStream       NativeCommand = "download_stream"
	CommandExtractDetectedAudio NativeCommand = "extract_detected_audio"
	CommandDownloadDetected     NativeCommand = "download_detected"
	CommandCancelJob            NativeCommand = "cancel_job"
	CommandCheckUpdate          NativeCommand = "check_update"
	CommandApplyUpdate          NativeCommand = "apply_update"
)

type JobState string

const (
	JobQueued      JobState = "queued"
	JobAnalyzing   JobState = "analyzing"
	JobDownloading JobState = "downloading"
	JobProcessing  JobState = "processing"
	JobCompleted   JobState = "completed"
	JobFailed      JobState = "failed"
	JobCancelled   JobState = "cancelled"
	JobInterrupted JobState = "interrupted"
)

type DownloadStrategy string

const (
	StrategyDirectHTTP DownloadStrategy = "direct_http"
	StrategyYTDLP      DownloadStrategy = "yt_dlp"
	StrategyFFmpeg     DownloadStrategy = "ffmpeg"
)

type ErrorCode string

const (
	ErrorInvalidRequest         ErrorCode = "invalid_request"
	ErrorUnsupported            ErrorCode = "unsupported"
	ErrorAuthenticationRequired ErrorCode = "authentication_required"
	ErrorUnavailable            ErrorCode = "unavailable"
	ErrorDRMProtected           ErrorCode = "drm_protected"
	ErrorExpiredURL             ErrorCode = "expired_url"
	ErrorHTTP403                ErrorCode = "http_403"
	ErrorExtractionFailed       ErrorCode = "extraction_failed"
	ErrorCancelled              ErrorCode = "cancelled"
	ErrorToolMissing            ErrorCode = "tool_missing"
	ErrorIO                     ErrorCode = "io_error"
	ErrorUpdateFailed           ErrorCode = "update_failed"
	ErrorInternal               ErrorCode = "internal"
)

type ErrorModel struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	Retryable  bool      `json:"retryable"`
	HTTPStatus int       `json:"httpStatus,omitempty"`
}
type CapabilityFlags struct {
	DirectHTTP      bool `json:"directHttp"`
	YTDLP           bool `json:"ytDlp"`
	FFmpeg          bool `json:"ffmpeg"`
	FFprobe         bool `json:"ffprobe"`
	Deno            bool `json:"deno"`
	Playlists       bool `json:"playlists"`
	Subtitles       bool `json:"subtitles"`
	Thumbnails      bool `json:"thumbnails"`
	BrowserSession  bool `json:"browserSession"`
	Cancel          bool `json:"cancel"`
	Retry           bool `json:"retry"`
	RestartRecovery bool `json:"restartRecovery"`
}
type MediaItem struct {
	ID          string  `json:"id,omitempty"`
	URL         string  `json:"url"`
	PageURL     string  `json:"pageUrl,omitempty"`
	Kind        string  `json:"kind"`
	Container   string  `json:"container,omitempty"`
	ContentType string  `json:"contentType,omitempty"`
	Source      string  `json:"source,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Bitrate     float64 `json:"bitrate,omitempty"`
	SizeBytes   int64   `json:"sizeBytes,omitempty"`
	SizeExact   bool    `json:"sizeExact,omitempty"`
	DirectSafe  bool    `json:"directSafe"`
	Protected   bool    `json:"protected"`
}
type DownloadRequest struct {
	URL       string           `json:"url,omitempty"`
	Path      string           `json:"path,omitempty"`
	JobID     string           `json:"jobId,omitempty"`
	Quality   string           `json:"quality,omitempty"`
	Bitrate   int              `json:"bitrate,omitempty"`
	Languages string           `json:"languages,omitempty"`
	Referer   string           `json:"referer,omitempty"`
	MediaType string           `json:"mediaType,omitempty"`
	Filename  string           `json:"filename,omitempty"`
	Playlist  bool             `json:"playlist,omitempty"`
	Strategy  DownloadStrategy `json:"strategy,omitempty"`
	UserAgent string           `json:"userAgent,omitempty"`
	Cookies   []BrowserCookie  `json:"cookies,omitempty"`
}
type Job struct {
	ID        string           `json:"id"`
	State     JobState         `json:"state"`
	Request   DownloadRequest  `json:"request"`
	Strategy  DownloadStrategy `json:"strategy,omitempty"`
	CreatedAt int64            `json:"createdAt"`
	UpdatedAt int64            `json:"updatedAt"`
}
type ProgressEvent struct {
	JobID           string   `json:"jobId"`
	State           JobState `json:"state"`
	Stage           string   `json:"stage,omitempty"`
	Progress        float64  `json:"progress"`
	SpeedBytes      float64  `json:"speedBytes,omitempty"`
	DownloadedBytes float64  `json:"downloadedBytes,omitempty"`
	TotalBytes      float64  `json:"totalBytes,omitempty"`
	ETASeconds      float64  `json:"etaSeconds,omitempty"`
	ElapsedSeconds  float64  `json:"elapsedSeconds,omitempty"`
}
type DownloadResult struct {
	JobID    string           `json:"jobId"`
	State    JobState         `json:"state"`
	Path     string           `json:"path"`
	Strategy DownloadStrategy `json:"strategy"`
}

type NativeRequest struct {
	ProtocolVersion int    `json:"protocolVersion,omitempty"`
	ID              string `json:"id,omitempty"`
	Action          string `json:"action,omitempty"`
	DownloadRequest
	Force    bool      `json:"force,omitempty"`
	Settings *Settings `json:"settings,omitempty"`
}
type NativeResponse struct {
	ProtocolVersion int              `json:"protocolVersion"`
	ID              string           `json:"id,omitempty"`
	Event           string           `json:"event"`
	JobID           string           `json:"jobId,omitempty"`
	Kind            string           `json:"kind,omitempty"`
	State           JobState         `json:"state,omitempty"`
	Strategy        DownloadStrategy `json:"strategy,omitempty"`
	Message         string           `json:"message,omitempty"`
	Details         string           `json:"details,omitempty"`
	Error           *ErrorModel      `json:"error,omitempty"`
	Capabilities    *CapabilityFlags `json:"capabilities,omitempty"`
	Stage           string           `json:"stage,omitempty"`
	Version         string           `json:"version,omitempty"`
	Path            string           `json:"path,omitempty"`
	Progress        float64          `json:"progress,omitempty"`
	SpeedBytes      float64          `json:"speedBytes,omitempty"`
	DownloadedBytes float64          `json:"downloadedBytes,omitempty"`
	TotalBytes      float64          `json:"totalBytes,omitempty"`
	ETASeconds      float64          `json:"etaSeconds,omitempty"`
	ElapsedSeconds  float64          `json:"elapsedSeconds,omitempty"`
	ProcessingRate  float64          `json:"processingRate,omitempty"`
	Settings        *Settings        `json:"settings,omitempty"`
	Tools           *ToolsStatus     `json:"tools,omitempty"`
	Info            *MediaInfo       `json:"info,omitempty"`
	Update          *UpdateStatus    `json:"update,omitempty"`
}
type Request = NativeRequest
type Response = NativeResponse

func stateForEvent(event, stage string) JobState {
	switch event {
	case "queued":
		return JobQueued
	case "job_started":
		return JobAnalyzing
	case "complete":
		return JobCompleted
	case "error":
		return JobFailed
	case "cancelled":
		return JobCancelled
	case "progress":
		if stage == "تحليل الرابط" {
			return JobAnalyzing
		}
		if stage == "تنزيل الوسائط" || stage == "تنزيل مباشر" {
			return JobDownloading
		}
		if stage == "دمج الفيديو والصوت" || stage == "تجهيز ملف MP3" || stage == "تحويل الصوت" || stage == "اكتمل" {
			return JobProcessing
		}
		return JobDownloading
	}
	return ""
}
func strategyForKind(kind string) DownloadStrategy {
	switch kind {
	case "download_detected":
		return StrategyDirectHTTP
	case "convert_mp3":
		return StrategyFFmpeg
	case "download_video", "download_audio", "download_thumbnail", "download_subtitles", "download_stream", "extract_detected_audio":
		return StrategyYTDLP
	}
	return ""
}
func protocolError(message string) *ErrorModel {
	return &ErrorModel{Code: ErrorInternal, Message: message, Retryable: true}
}
func capabilityFlags(tools ToolsStatus) CapabilityFlags {
	return CapabilityFlags{DirectHTTP: true, YTDLP: tools.YTDLP.Found, FFmpeg: tools.FFmpeg.Found, FFprobe: tools.FFprobe.Found, Deno: tools.Deno.Found, Playlists: tools.YTDLP.Found, Subtitles: tools.YTDLP.Found, Thumbnails: tools.YTDLP.Found, BrowserSession: true, Cancel: true, Retry: true, RestartRecovery: true}
}
func validCommand(command string) bool {
	switch NativeCommand(command) {
	case CommandPing, CommandGetSettings, CommandSaveSettings, CommandCheckTools, CommandPickFile, CommandPickOutputFolder, CommandOpenPath, CommandFetchInfo, CommandConvertMP3, CommandDownloadVideo, CommandDownloadAudio, CommandDownloadThumbnail, CommandDownloadSubtitles, CommandDownloadStream, CommandExtractDetectedAudio, CommandDownloadDetected, CommandCancelJob, CommandCheckUpdate, CommandApplyUpdate:
		return true
	}
	return false
}

func validEvent(event string) bool {
	switch event {
	case "pong", "settings", "settings_saved", "tools_status", "file_cancelled", "file_selected", "folder_cancelled", "folder_selected", "path_opened", "media_info", "queued", "job_started", "progress", "complete", "error", "cancelled", "cancel_requested", "update_status", "update_error", "update_progress", "update_restarting":
		return true
	}
	return false
}
