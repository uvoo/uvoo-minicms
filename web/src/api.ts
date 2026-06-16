export type Page = {
  id:number
  slug:string
  path:string
  title:string
  meta_description:string
  content_type:'page'|'post'
  markdown?:string
  tags:string
  published:boolean
  published_at:string
  created_at:string
  updated_at:string
}
export type PageRevision = {
  id:number
  page_id:number
  slug:string
  path:string
  title:string
  meta_description:string
  content_type:'page'|'post'
  markdown:string
  tags:string
  published:boolean
  published_at:string
  created_at:string
}
export type NavItem = { id:string; type?:'link'|'section'; parent_id:string; label:string; url:string; external:boolean; enabled:boolean }
export type Asset = { id:number; name:string; url:string; size:number; created_at:string }
export type ACLRule = { id?:number; scope:'all'|'admin'|'public'; action:'allow'|'deny'; cidr:string; note:string; enabled:boolean }
export type ACLSettings = {
  admin_default:'allow'|'deny'
  public_default:'allow'|'deny'
  admin_allow_countries:string
  admin_deny_countries:string
  public_allow_countries:string
  public_deny_countries:string
  rules:ACLRule[]
}
export type SiteSettings = {
  site_name:string
  logo_url:string
  favicon_url:string
  default_theme:'light'|'dark'
  public_theme_style:'soft'|'square'|'material'
  public_primary_color:string
  public_secondary_color:string
  public_header_style:'neutral'|'accent-line'|'accent-bg'
  admin_theme:'light'|'dark'
  theme_style:'soft'|'square'|'material'
  admin_primary_color:string
  admin_secondary_color:string
  admin_palette:'slate'|'forest'|'ember'|'mono'|'custom'
  footer_markdown:string
  menu:NavItem[]
  logo_enabled:boolean
  favicon_enabled:boolean
  menu_enabled:boolean
  footer_enabled:boolean
  theme_toggle_enabled:boolean
  icons_enabled:boolean
  search_enabled:boolean
  nav_layout:'top'|'side'
  blog_enabled:boolean
  blog_path:string
  blog_title:string
  blog_menu_enabled:boolean
  blog_posts_per_page:number
  revision_history_limit:number
}
export type ThemeHistory = {
  id:number
  admin_theme:'light'|'dark'
  theme_style:'soft'|'square'|'material'
  admin_primary_color:string
  admin_secondary_color:string
  admin_palette:'slate'|'forest'|'ember'|'mono'|'custom'
  public_theme:'light'|'dark'
  public_theme_style:'soft'|'square'|'material'
  public_primary_color:string
  public_secondary_color:string
  public_header_style:'neutral'|'accent-line'|'accent-bg'
  updated_at:string
}
export type SessionInfo = {
  username:string
  auth_scheme:string
  jwt_source?:string
  jwt_claims?:Record<string, unknown>
  proxy_identity?:Record<string, unknown>
}
export type ImportPage = {
  slug:string
  path:string
  title:string
  meta_description:string
  content_type:'page'|'post'
  tags:string
  markdown:string
  source_url:string
  published:boolean
  exists:boolean
}
export type ImportOptions = {
  url:string
  max_pages:number
  include_posts:boolean
  import_menu:boolean
  publish:boolean
  update_existing:boolean
  download_images:boolean
  advanced_scraping:boolean
}
export type ImportResult = {
  source:string
  base_url:string
  pages:ImportPage[]
  menu:NavItem[]
  imported:number
  skipped:number
  errors:string[]
  existing:number
  wordpress:boolean
  sitemap_url:string
  preview_limit:number
}

const baseURL = new URL('/cms.v1.CMSService/', window.location.href)
baseURL.username = ''
baseURL.password = ''
async function rpc<T>(name: string, body: Record<string, unknown> = {}): Promise<T> {
  const r = await fetch(new URL(name, baseURL).toString(), { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}
export const api = {
  session: () => rpc<{session:SessionInfo}>('Session'),
  listPages: () => rpc<{pages:Page[]}>('ListPages'),
  getPage: (slug:string) => rpc<{page:Page}>('GetPage', { slug }),
  savePage: (page: Partial<Page>) => rpc<{page:Page}>('SavePage', page),
  listPageRevisions: (slug:string) => rpc<{revisions:PageRevision[]}>('ListPageRevisions', { slug }),
  deletePage: (slug:string) => rpc<{ok:boolean}>('DeletePage', { slug }),
  getSettings: () => rpc<{settings:SiteSettings}>('GetSettings'),
  saveSettings: (settings: SiteSettings) => rpc<{settings:SiteSettings}>('SaveSettings', settings),
  listThemeHistory: () => rpc<{themes:ThemeHistory[]}>('ListThemeHistory'),
  listAssets: () => rpc<{assets:Asset[]}>('ListAssets'),
  uploadFile: (name:string, data:string) => rpc<{asset:Asset}>('UploadFile', { name, data }),
  deleteAsset: (id:number) => rpc<{ok:boolean, settings:SiteSettings}>('DeleteAsset', { id }),
  setSiteImage: (kind:'logo'|'favicon', name:string, data:string, url = '') => rpc<{asset:Asset, settings:SiteSettings}>('SetSiteImage', { kind, name, data, url }),
  getACL: () => rpc<{acl:ACLSettings}>('GetACL'),
  saveACL: (acl: ACLSettings) => rpc<{acl:ACLSettings}>('SaveACL', acl),
  importPreview: (opts: ImportOptions) => rpc<{import:ImportResult}>('ImportPreview', opts),
  importSite: (opts: ImportOptions) => rpc<{import:ImportResult}>('ImportSite', opts)
}
