package web

const (
	statusCodeReady                                 = "ready"
	statusCodeRuntimeReady                          = "runtime_ready"
	statusCodeNoLocalModelSelectDownload            = "no_local_model_select_download"
	statusCodeRuntimeIdleLoadOnFirstTranslation     = "runtime_idle_load_on_first_translation"
	statusCodeTranslationCompleted                  = "translation_completed"
	statusCodeStreamingTranslation                  = "streaming_translation"
	statusCodeFileTranslationCompleted              = "file_translation_completed"
	statusCodeHistoryItemDeleted                    = "history_item_deleted"
	statusCodeHistoryCleared                        = "history_cleared"
	statusCodeModelInstalledLoadOnFirstTranslation  = "model_installed_load_on_first_translation"
	statusCodeModelActive                           = "model_active"
	statusCodeVisionRuntimeActive                   = "vision_runtime_active"
	statusCodeModelDeleted                          = "model_deleted"
	statusCodeModelNotInstalledLocally              = "model_not_installed_locally"
	statusCodeUnknownModelSelection                 = "unknown_model_selection"
	statusCodeInvalidFormPayload                    = "invalid_form_payload"
	statusCodeMissingHistoryID                      = "missing_history_id"
	statusCodeInvalidHistoryID                      = "invalid_history_id"
	statusCodeHistoryItemNotFound                   = "history_item_not_found"
	statusCodeVisionRuntimeUnavailable              = "vision_runtime_unavailable"
	statusCodePreparingModelInstall                 = "preparing_model_install"
	statusCodeSwitchingActiveModel                  = "switching_active_model"
	statusCodeRemovingLocalModel                    = "removing_local_model"
	statusCodeDownloadingVisionRuntime              = "downloading_vision_runtime"
	statusCodePreparingActiveModel                  = "preparing_active_model"
	statusCodeActiveRuntimeNoImageSupport           = "active_runtime_no_image_support"
	statusCodeInvalidMultipartPayloadOrFileTooLarge = "invalid_multipart_payload_or_file_too_large"
	statusCodeMissingImageFile                      = "missing_image_file"
	statusCodeUnableToReadImageFile                 = "unable_to_read_image_file"
	statusCodeImageFileEmpty                        = "image_file_empty"
	statusCodeImageFileExceedsSizeLimit             = "image_file_exceeds_size_limit"
	statusCodeUnsupportedImageFormat                = "unsupported_image_format"
)

func makeStatus(code, message string) uiStatus {
	return uiStatus{Code: code, Message: message}
}

func makeServerStatus(code, message string) uiStatus {
	return makeStatus(code, message)
}

func makeRuntimeStatus(code, message string) uiStatus {
	return makeStatus(code, message)
}
