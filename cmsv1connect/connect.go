package cmsv1connect

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
)

const CMSServiceName = "cms.v1.CMSService"

const (
	CMSServiceHealthProcedure            = "/cms.v1.CMSService/Health"
	CMSServiceSessionProcedure           = "/cms.v1.CMSService/Session"
	CMSServiceListPagesProcedure         = "/cms.v1.CMSService/ListPages"
	CMSServiceGetPageProcedure           = "/cms.v1.CMSService/GetPage"
	CMSServiceSavePageProcedure          = "/cms.v1.CMSService/SavePage"
	CMSServiceListPageRevisionsProcedure = "/cms.v1.CMSService/ListPageRevisions"
	CMSServiceDeletePageProcedure        = "/cms.v1.CMSService/DeletePage"
	CMSServiceGetSettingsProcedure       = "/cms.v1.CMSService/GetSettings"
	CMSServiceSaveSettingsProcedure      = "/cms.v1.CMSService/SaveSettings"
	CMSServiceListThemeHistoryProcedure  = "/cms.v1.CMSService/ListThemeHistory"
	CMSServiceListAssetsProcedure        = "/cms.v1.CMSService/ListAssets"
	CMSServiceUploadFileProcedure        = "/cms.v1.CMSService/UploadFile"
	CMSServiceDeleteAssetProcedure       = "/cms.v1.CMSService/DeleteAsset"
	CMSServiceSetSiteImageProcedure      = "/cms.v1.CMSService/SetSiteImage"
	CMSServiceGetACLProcedure            = "/cms.v1.CMSService/GetACL"
	CMSServiceSaveACLProcedure           = "/cms.v1.CMSService/SaveACL"
	CMSServiceImportPreviewProcedure     = "/cms.v1.CMSService/ImportPreview"
	CMSServiceImportSiteProcedure        = "/cms.v1.CMSService/ImportSite"
)

type CMSServiceHandler interface {
	Health(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	Session(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	ListPages(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	GetPage(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	SavePage(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	ListPageRevisions(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	DeletePage(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	GetSettings(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	SaveSettings(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	ListThemeHistory(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	ListAssets(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	UploadFile(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	DeleteAsset(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	SetSiteImage(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	GetACL(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	SaveACL(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	ImportPreview(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
	ImportSite(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error)
}

func NewCMSServiceHandler(svc CMSServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	mux := http.NewServeMux()
	mux.Handle(CMSServiceHealthProcedure, connect.NewUnaryHandler(CMSServiceHealthProcedure, svc.Health, opts...))
	mux.Handle(CMSServiceSessionProcedure, connect.NewUnaryHandler(CMSServiceSessionProcedure, svc.Session, opts...))
	mux.Handle(CMSServiceListPagesProcedure, connect.NewUnaryHandler(CMSServiceListPagesProcedure, svc.ListPages, opts...))
	mux.Handle(CMSServiceGetPageProcedure, connect.NewUnaryHandler(CMSServiceGetPageProcedure, svc.GetPage, opts...))
	mux.Handle(CMSServiceSavePageProcedure, connect.NewUnaryHandler(CMSServiceSavePageProcedure, svc.SavePage, opts...))
	mux.Handle(CMSServiceListPageRevisionsProcedure, connect.NewUnaryHandler(CMSServiceListPageRevisionsProcedure, svc.ListPageRevisions, opts...))
	mux.Handle(CMSServiceDeletePageProcedure, connect.NewUnaryHandler(CMSServiceDeletePageProcedure, svc.DeletePage, opts...))
	mux.Handle(CMSServiceGetSettingsProcedure, connect.NewUnaryHandler(CMSServiceGetSettingsProcedure, svc.GetSettings, opts...))
	mux.Handle(CMSServiceSaveSettingsProcedure, connect.NewUnaryHandler(CMSServiceSaveSettingsProcedure, svc.SaveSettings, opts...))
	mux.Handle(CMSServiceListThemeHistoryProcedure, connect.NewUnaryHandler(CMSServiceListThemeHistoryProcedure, svc.ListThemeHistory, opts...))
	mux.Handle(CMSServiceListAssetsProcedure, connect.NewUnaryHandler(CMSServiceListAssetsProcedure, svc.ListAssets, opts...))
	mux.Handle(CMSServiceUploadFileProcedure, connect.NewUnaryHandler(CMSServiceUploadFileProcedure, svc.UploadFile, opts...))
	mux.Handle(CMSServiceDeleteAssetProcedure, connect.NewUnaryHandler(CMSServiceDeleteAssetProcedure, svc.DeleteAsset, opts...))
	mux.Handle(CMSServiceSetSiteImageProcedure, connect.NewUnaryHandler(CMSServiceSetSiteImageProcedure, svc.SetSiteImage, opts...))
	mux.Handle(CMSServiceGetACLProcedure, connect.NewUnaryHandler(CMSServiceGetACLProcedure, svc.GetACL, opts...))
	mux.Handle(CMSServiceSaveACLProcedure, connect.NewUnaryHandler(CMSServiceSaveACLProcedure, svc.SaveACL, opts...))
	mux.Handle(CMSServiceImportPreviewProcedure, connect.NewUnaryHandler(CMSServiceImportPreviewProcedure, svc.ImportPreview, opts...))
	mux.Handle(CMSServiceImportSiteProcedure, connect.NewUnaryHandler(CMSServiceImportSiteProcedure, svc.ImportSite, opts...))
	return "/cms.v1.CMSService/", mux
}
