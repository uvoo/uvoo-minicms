import React, { useEffect, useMemo, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { App as AntApp, Button, Card, ConfigProvider, Dropdown, Form, Input, Layout, List, Modal, Popconfirm, Select, Space, Switch, Tabs, Typography, Upload, message, theme } from 'antd'
import type { MenuProps, UploadProps } from 'antd'
import './style.css'
import { api, ACLRule, ACLSettings, Asset, ImportOptions, ImportResult, NavItem, Page, PageRevision, SessionInfo, SiteSettings, ThemeHistory } from './api'

const MdBodyEditor = React.lazy(() => import('./MdBodyEditor'))
const adminThemeStorageKey = 'uvoo-minicms-admin-theme'

const palettes = {
  slate: { colorPrimary: '#2563eb', colorBgLayout: '#f4f7fb', colorText: '#172033', colorBorder: '#d8dee9' },
  forest: { colorPrimary: '#3b7a57', colorBgLayout: '#f7f5ef', colorText: '#263238', colorBorder: '#e7e0d2' },
  ember: { colorPrimary: '#b45309', colorBgLayout: '#fff7ed', colorText: '#292524', colorBorder: '#fed7aa' },
  mono: { colorPrimary: '#111827', colorBgLayout: '#f9fafb', colorText: '#111827', colorBorder: '#e5e7eb' }
}

type Palette = keyof typeof palettes | 'custom'
type IdentityKind = 'logo' | 'favicon'
type ThemeStyle = 'soft' | 'square' | 'material'

function slugify(s:string) { return s.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') }
function pathify(s:string) {
  if (/^https?:\/\//i.test(s) || /^mailto:/i.test(s) || /^tel:/i.test(s)) return s.trim()
  const path = s.split('/').map(slugify).filter(Boolean).join('/')
  return path ? `/${path}` : '/'
}
function joinPath(base:string, slug:string) {
  const cleanBase = pathify(base || '/blog').replace(/\/+$/, '')
  return `${cleanBase || ''}/${slug}` || `/${slug}`
}
function freshPage() {
  return { slug:'', path:'', title:'', meta_description:'', content_type:'page', tags:'', markdown:'# Untitled\n', published:false, published_at:'' } as Page
}
function defaultFooter(siteName = 'Uvoo-MiniCMS') {
  return `© ${new Date().getUTCFullYear()} ${siteName}. All rights reserved.`
}
function newID() {
  return globalThis.crypto?.randomUUID?.() || `item-${Date.now()}-${Math.random().toString(16).slice(2)}`
}
function isMenuDescendant(items: NavItem[], itemID: string, ancestorID: string) {
  const byID = new Map(items.filter(item => item?.id).map(item => [item.id, item]))
  const seen = new Set<string>()
  let parentID = byID.get(itemID)?.parent_id || ''
  while (parentID) {
    if (parentID === ancestorID) return true
    if (seen.has(parentID)) return false
    seen.add(parentID)
    parentID = byID.get(parentID)?.parent_id || ''
  }
  return false
}
function menuParentOptions(items: NavItem[], rowIndex: number) {
  const currentID = items[rowIndex]?.id || ''
  return items
    .filter((item, i) => i !== rowIndex && item?.id && (!currentID || !isMenuDescendant(items, item.id, currentID)))
    .map(item => ({ label: item.label || item.url || item.id, value: item.id }))
}
function menuSearchValue(item?: NavItem) {
  if (!item) return ''
  return [item.label, item.url, item.id, item.parent_id, item.type].filter(Boolean).join(' ').toLowerCase()
}
function menuSortLabel(item: NavItem) {
  return (item.label || item.url || item.id || '').trim().toLocaleLowerCase()
}
function sortMenuAlphabetically(items: NavItem[]) {
  const rows = items.map((item, index) => ({ item, index }))
  const children = new Map<string, typeof rows>()
  rows.forEach(row => {
    const parentID = row.item.parent_id || ''
    children.set(parentID, [...(children.get(parentID) || []), row])
  })
  children.forEach(list => list.sort((a, b) => {
    const byLabel = menuSortLabel(a.item).localeCompare(menuSortLabel(b.item))
    return byLabel || a.index - b.index
  }))
  const out: NavItem[] = []
  const seen = new Set<string>()
  const append = (parentID: string) => {
    for (const row of children.get(parentID) || []) {
      out.push(row.item)
      if (row.item.id && !seen.has(row.item.id)) {
        seen.add(row.item.id)
        append(row.item.id)
      }
    }
  }
  append('')
  for (const row of rows) {
    if (!out.includes(row.item)) out.push(row.item)
  }
  return out
}
function siblingMenuIndex(items: NavItem[], index: number, direction: -1 | 1) {
  const parentID = items[index]?.parent_id || ''
  for (let i = index + direction; i >= 0 && i < items.length; i += direction) {
    if ((items[i]?.parent_id || '') === parentID) return i
  }
  return -1
}
function hexToRgb(hex:string) {
  const cleaned = hex.replace('#', '')
  const value = /^[0-9a-fA-F]{6}$/.test(cleaned) ? cleaned : '386bc0'
  return `${parseInt(value.slice(0, 2), 16)}, ${parseInt(value.slice(2, 4), 16)}, ${parseInt(value.slice(4, 6), 16)}`
}
function themeRadius(style: ThemeStyle) {
  if (style === 'square') return 2
  if (style === 'material') return 4
  return 8
}
function themeLabel(theme: ThemeHistory) {
  return `${theme.admin_primary_color} / ${theme.public_primary_color} · ${theme.theme_style}`
}
function isImage(url:string) {
  return /\.(avif|gif|jpe?g|png|webp)$/i.test(url)
}
function assetMarkdown(asset: Asset) {
  return isImage(asset.url) ? `![${asset.name}](${asset.url})` : `[${asset.name}](${asset.url})`
}
function storedAdminDark() {
  return storedAdminTheme() !== 'light'
}
function storedAdminTheme(): 'light'|'dark'|'' {
  try {
    const value = localStorage.getItem(adminThemeStorageKey)
    return value === 'light' || value === 'dark' ? value : ''
  } catch {
    return ''
  }
}
function rememberAdminTheme(dark: boolean) {
  try {
    localStorage.setItem(adminThemeStorageKey, dark ? 'dark' : 'light')
  } catch {
    // Ignore private browsing or storage policy failures.
  }
}
function displayValue(value: unknown) {
  if (Array.isArray(value)) return value.join(', ')
  if (value === null || value === undefined) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}
function readFileData(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(file)
  })
}
const emptyACL: ACLSettings = {
  admin_default: 'allow',
  public_default: 'allow',
  admin_allow_countries: '',
  admin_deny_countries: '',
  public_allow_countries: '',
  public_deny_countries: '',
  rules: []
}
const themeStyleOptions = [
  { label: 'Soft', value: 'soft' },
  { label: 'Square', value: 'square' },
  { label: 'Material', value: 'material' }
]

function Root() {
  const [pages, setPages] = useState<Page[]>([])
  const [assets, setAssets] = useState<Asset[]>([])
  const [acl, setACL] = useState<ACLSettings>(emptyACL)
  const [themeHistory, setThemeHistory] = useState<ThemeHistory[]>([])
  const [session, setSession] = useState<SessionInfo | null>(null)
  const [active, setActive] = useState<Page | null>(null)
  const [saving, setSaving] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const [savingACL, setSavingACL] = useState(false)
  const [loadingAssets, setLoadingAssets] = useState(false)
  const [mediaOpen, setMediaOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [pageRevisions, setPageRevisions] = useState<PageRevision[]>([])
  const [loadingRevisions, setLoadingRevisions] = useState(false)
  const [sourceMode, setSourceMode] = useState(false)
  const [editorRev, setEditorRev] = useState(0)
  const [palette, setPalette] = useState<Palette>('slate')
  const [customPrimary, setCustomPrimary] = useState('#386bc0')
  const [customSecondary, setCustomSecondary] = useState('#64748b')
  const [adminDark, setAdminDark] = useState(storedAdminDark)
  const [themeStyle, setThemeStyle] = useState<ThemeStyle>('soft')
  const [publicTheme, setPublicTheme] = useState<'light'|'dark'>('dark')
  const [publicThemeStyle, setPublicThemeStyle] = useState<ThemeStyle>('soft')
  const [publicPrimary, setPublicPrimary] = useState('#386bc0')
  const [publicSecondary, setPublicSecondary] = useState('#64748b')
  const [publicHeaderStyle, setPublicHeaderStyle] = useState<'neutral'|'accent-line'|'accent-bg'>('neutral')
  const [footerEnabled, setFooterEnabled] = useState(true)
  const [footerMarkdown, setFooterMarkdown] = useState(defaultFooter())
  const [importURL, setImportURL] = useState('')
  const [importMaxPages, setImportMaxPages] = useState(50)
  const [importIncludePosts, setImportIncludePosts] = useState(true)
  const [importMenu, setImportMenu] = useState(true)
  const [importPublish, setImportPublish] = useState(true)
  const [importUpdateExisting, setImportUpdateExisting] = useState(false)
  const [importDownloadImages, setImportDownloadImages] = useState(true)
  const [importAdvancedScraping, setImportAdvancedScraping] = useState(false)
  const [importPreview, setImportPreview] = useState<ImportResult | null>(null)
  const [previewingImport, setPreviewingImport] = useState(false)
  const [runningImport, setRunningImport] = useState(false)
  const [identityUploading, setIdentityUploading] = useState<IdentityKind | ''>('')
  const [identitySourceURL, setIdentitySourceURL] = useState<Record<IdentityKind, string>>({ logo: '', favicon: '' })
  const [menuSearch, setMenuSearch] = useState('')
  const [form] = Form.useForm()
  const [settingsForm] = Form.useForm<SiteSettings>()
  const md = Form.useWatch('markdown', form) ?? ''
  const menuItems = Form.useWatch('menu', settingsForm) || []
  const logoURL = Form.useWatch('logo_url', settingsForm) || ''
  const faviconURL = Form.useWatch('favicon_url', settingsForm) || ''
  const selectedPalette = palette === 'custom'
    ? { ...palettes.slate, colorPrimary: customPrimary }
    : palettes[palette]
  const adminTokens = {
    colorPrimary: selectedPalette.colorPrimary,
    colorBgLayout: adminDark ? '#0f172a' : selectedPalette.colorBgLayout,
    colorBgContainer: adminDark ? '#172033' : '#ffffff',
    colorBgElevated: adminDark ? '#1f2a3d' : '#ffffff',
    colorText: adminDark ? '#e5edf8' : selectedPalette.colorText,
    colorTextSecondary: adminDark ? '#9fb0c7' : '#657085',
    colorBorder: adminDark ? '#2c3b52' : selectedPalette.colorBorder,
    borderRadius: themeRadius(themeStyle),
    boxShadow: themeStyle === 'material' ? '0 3px 8px rgba(15, 23, 42, .22)' : undefined
  }
  const cfg = useMemo(() => ({
    token: adminTokens,
    algorithm: adminDark ? theme.darkAlgorithm : theme.defaultAlgorithm
  }), [adminTokens, adminDark])
  const adminVars = {
    '--admin-primary': adminTokens.colorPrimary,
    '--admin-primary-rgb': hexToRgb(adminTokens.colorPrimary),
    '--admin-secondary': customSecondary,
    '--admin-secondary-rgb': hexToRgb(customSecondary),
    '--admin-bg': adminTokens.colorBgLayout,
    '--admin-bg-soft': adminDark ? '#111827' : '#eaf1fb',
    '--admin-surface': adminTokens.colorBgContainer,
    '--admin-surface-2': adminDark ? '#1f2a3d' : '#f8fafc',
    '--admin-text': adminTokens.colorText,
    '--admin-muted': adminTokens.colorTextSecondary,
    '--admin-border': adminTokens.colorBorder,
    '--admin-radius': themeStyle === 'square' ? '4px' : themeStyle === 'material' ? '8px' : '20px',
    '--admin-control-radius': `${themeRadius(themeStyle)}px`,
    '--admin-shadow': themeStyle === 'material' ? (adminDark ? '#00000070' : '#17203326') : (adminDark ? '#00000055' : '#17203312')
  } as React.CSSProperties
  const imageSuggestions = assets.filter(asset => isImage(asset.url)).map(asset => asset.url)
  const menuSearchQuery = menuSearch.trim().toLowerCase()

  function setAdminThemeMode(mode: 'light'|'dark') {
    const dark = mode === 'dark'
    setAdminDark(dark)
    rememberAdminTheme(dark)
    settingsForm.setFieldValue('admin_theme', mode)
  }
  function setMenuItems(items: NavItem[]) {
    settingsForm.setFieldValue('menu', items)
  }
  function sortCurrentMenuAlphabetically() {
    setMenuItems(sortMenuAlphabetically((settingsForm.getFieldValue('menu') || []) as NavItem[]))
  }
  function moveMenuItem(index: number, direction: -1 | 1) {
    const items = [...((settingsForm.getFieldValue('menu') || []) as NavItem[])]
    const swapIndex = siblingMenuIndex(items, index, direction)
    if (swapIndex < 0) return
    const current = items[index]
    items[index] = items[swapIndex]
    items[swapIndex] = current
    setMenuItems(items)
  }

  async function loadPages() {
    const r = await api.listPages()
    setPages(r.pages)
    if (!active && r.pages[0]) openPage(r.pages[0].slug)
  }
  async function loadSettings() {
    const r = await api.getSettings()
    const savedAdminTheme = r.settings.admin_theme === 'light' ? 'light' : 'dark'
    const adminTheme = storedAdminTheme() || savedAdminTheme
    settingsForm.setFieldsValue({ ...r.settings, admin_theme: adminTheme })
    setAdminThemeMode(adminTheme)
    setCustomPrimary(r.settings.admin_primary_color || '#386bc0')
    setCustomSecondary(r.settings.admin_secondary_color || '#64748b')
    if (r.settings.admin_palette) setPalette(r.settings.admin_palette)
    setThemeStyle(r.settings.theme_style || 'soft')
    setPublicTheme(r.settings.default_theme === 'dark' ? 'dark' : 'light')
    setPublicThemeStyle(r.settings.public_theme_style || 'soft')
    setPublicPrimary(r.settings.public_primary_color || '#386bc0')
    setPublicSecondary(r.settings.public_secondary_color || '#64748b')
    setPublicHeaderStyle(r.settings.public_header_style || 'neutral')
    setFooterEnabled(r.settings.footer_enabled !== false)
    setFooterMarkdown(r.settings.footer_markdown || defaultFooter(r.settings.site_name))
  }
  async function loadSession() {
    const r = await api.session()
    setSession(r.session)
  }
  async function loadAssets() {
    setLoadingAssets(true)
    try {
      const r = await api.listAssets()
      setAssets(r.assets)
    } finally {
      setLoadingAssets(false)
    }
  }
  async function loadACL() {
    const r = await api.getACL()
    setACL({ ...emptyACL, ...r.acl, rules: r.acl.rules || [] })
  }
  async function loadThemeHistory() {
    const r = await api.listThemeHistory()
    setThemeHistory(r.themes || [])
  }
  async function openPage(slug:string) {
    const r = await api.getPage(slug)
    setActive(r.page)
    form.setFieldsValue(r.page)
    setEditorRev(rev => rev + 1)
  }
  async function loadRevisions(slug:string) {
    setLoadingRevisions(true)
    setHistoryOpen(true)
    try {
      const r = await api.listPageRevisions(slug)
      setPageRevisions(r.revisions || [])
    } catch(e:any) {
      message.error(e.message)
    } finally {
      setLoadingRevisions(false)
    }
  }
  function restoreRevision(revision: PageRevision) {
    form.setFieldsValue({
      slug: revision.slug,
      path: revision.path,
      title: revision.title,
      meta_description: revision.meta_description,
      content_type: revision.content_type,
      tags: revision.tags,
      markdown: revision.markdown,
      published: revision.published,
      published_at: revision.published_at
    })
    setSourceMode(true)
    setEditorRev(rev => rev + 1)
    setHistoryOpen(false)
    message.success('Revision loaded into editor')
  }
  async function savePage() {
    setSaving(true)
    try {
      const v = form.getFieldsValue()
      const slug = slugify(v.slug || v.title || '')
      const rootPath = slug ? `/${slug}` : ''
      const generatedPath = v.content_type === 'post' && (!v.path || pathify(v.path) === rootPath)
        ? joinPath(settingsForm.getFieldValue('blog_path') || '/blog', slug)
        : pathify(v.path || v.slug || v.title || '')
      const r = await api.savePage({ ...v, slug, path: generatedPath })
      setActive(r.page)
      form.setFieldsValue(r.page)
      setEditorRev(rev => rev + 1)
      await loadPages()
      message.success('Page saved')
    } catch(e:any) {
      message.error(e.message)
    } finally {
      setSaving(false)
    }
  }
  async function saveSettings(overrides: Partial<SiteSettings> = {}) {
    setSavingSettings(true)
    try {
      const values = { ...settingsForm.getFieldsValue(true), ...overrides }
      const menu = ((values.menu || []) as NavItem[]).map(item => {
        const itemType = item.type === 'section' ? 'section' : 'link'
        return {
          ...item,
          id: item.id || newID(),
          type: itemType,
          parent_id: item.parent_id || '',
          url: itemType === 'section' ? '' : pathify(item.url || ''),
          external: itemType === 'section' ? false : item.external,
          enabled: item.enabled !== false
        }
      }).filter(item => item.label && (item.type === 'section' || item.url))
      const r = await api.saveSettings({
        ...values,
        default_theme: values.default_theme || publicTheme,
        public_primary_color: values.public_primary_color || publicPrimary,
        public_secondary_color: values.public_secondary_color || publicSecondary,
        public_header_style: values.public_header_style || publicHeaderStyle,
        admin_theme: adminDark ? 'dark' : 'light',
        theme_style: themeStyle,
        admin_primary_color: adminTokens.colorPrimary,
        admin_secondary_color: customSecondary,
        admin_palette: palette,
        public_theme_style: values.public_theme_style || publicThemeStyle,
        menu
      } as SiteSettings)
      settingsForm.setFieldsValue(r.settings)
      setPublicTheme(r.settings.default_theme === 'dark' ? 'dark' : 'light')
      setPublicThemeStyle(r.settings.public_theme_style || 'soft')
      setPublicPrimary(r.settings.public_primary_color || '#386bc0')
      setPublicSecondary(r.settings.public_secondary_color || '#64748b')
      setPublicHeaderStyle(r.settings.public_header_style || 'neutral')
      setFooterEnabled(r.settings.footer_enabled !== false)
      setFooterMarkdown(r.settings.footer_markdown || defaultFooter(r.settings.site_name))
      setCustomSecondary(r.settings.admin_secondary_color || '#64748b')
      setThemeStyle(r.settings.theme_style || 'soft')
      await loadThemeHistory()
      message.success('Site settings saved')
    } catch(e:any) {
      message.error(e.message)
    } finally {
      setSavingSettings(false)
    }
  }
  async function saveACL() {
    setSavingACL(true)
    try {
      const clean: ACLSettings = {
        ...acl,
        rules: acl.rules.map(rule => ({
          ...rule,
          cidr: rule.cidr.trim(),
          note: rule.note.trim(),
          enabled: rule.enabled !== false
        })).filter(rule => rule.cidr)
      }
      const r = await api.saveACL(clean)
      setACL({ ...emptyACL, ...r.acl, rules: r.acl.rules || [] })
      message.success('Security rules saved')
    } catch(e:any) {
      message.error(e.message)
    } finally {
      setSavingACL(false)
    }
  }
  function setACLField<K extends keyof ACLSettings>(key: K, value: ACLSettings[K]) {
    setACL(current => ({ ...current, [key]: value }))
  }
  function addACLRule() {
    setACL(current => ({ ...current, rules: [...current.rules, { scope: 'admin', action: 'allow', cidr: '', note: '', enabled: true }] }))
  }
  function updateACLRule(index:number, patch: Partial<ACLRule>) {
    setACL(current => ({ ...current, rules: current.rules.map((rule, i) => i === index ? { ...rule, ...patch } : rule) }))
  }
  function removeACLRule(index:number) {
    setACL(current => ({ ...current, rules: current.rules.filter((_, i) => i !== index) }))
  }
  function applyThemeHistory(theme: ThemeHistory) {
    setAdminThemeMode(theme.admin_theme === 'dark' ? 'dark' : 'light')
    setThemeStyle(theme.theme_style || 'soft')
    setPalette(theme.admin_palette || 'custom')
    setCustomPrimary(theme.admin_primary_color)
    setCustomSecondary(theme.admin_secondary_color)
    setPublicTheme(theme.public_theme === 'dark' ? 'dark' : 'light')
    setPublicThemeStyle(theme.public_theme_style || 'soft')
    setPublicPrimary(theme.public_primary_color)
    setPublicSecondary(theme.public_secondary_color)
    setPublicHeaderStyle(theme.public_header_style || 'neutral')
    settingsForm.setFieldsValue({
      admin_theme: theme.admin_theme,
      theme_style: theme.theme_style,
      admin_primary_color: theme.admin_primary_color,
      admin_secondary_color: theme.admin_secondary_color,
      admin_palette: theme.admin_palette,
      default_theme: theme.public_theme,
      public_theme_style: theme.public_theme_style,
      public_primary_color: theme.public_primary_color,
      public_secondary_color: theme.public_secondary_color,
      public_header_style: theme.public_header_style
    })
  }
  async function removePage(slug:string) {
    await api.deletePage(slug)
    setActive(null)
    form.resetFields()
    await loadPages()
  }
  function newPage(contentType: 'page'|'post' = 'page') {
    const p = freshPage()
    p.content_type = contentType
    if (contentType === 'post') {
      p.markdown = '# New Post\n'
    }
    setActive(p)
    form.setFieldsValue(p)
    setSourceMode(false)
    setEditorRev(rev => rev + 1)
  }
  function updateContentType(contentType: 'page'|'post') {
    form.setFieldValue('content_type', contentType)
    const slug = slugify(form.getFieldValue('slug') || form.getFieldValue('title') || '')
    const currentPath = form.getFieldValue('path') || ''
    if (contentType === 'post' && slug && (!currentPath || currentPath === `/${slug}`)) {
      form.setFieldValue('path', joinPath(settingsForm.getFieldValue('blog_path') || '/blog', slug))
    }
  }
  function currentImportOptions(): ImportOptions {
    return {
      url: importURL,
      max_pages: importMaxPages,
      include_posts: importIncludePosts,
      import_menu: importMenu,
      publish: importPublish,
      update_existing: importUpdateExisting,
      download_images: importDownloadImages,
      advanced_scraping: importAdvancedScraping
    }
  }
  async function previewImportSite() {
    setPreviewingImport(true)
    try {
      const r = await api.importPreview(currentImportOptions())
      setImportPreview(r.import)
      message.success(`Found ${r.import.pages.length} page(s)`)
    } catch(e:any) {
      message.error(e.message)
    } finally {
      setPreviewingImport(false)
    }
  }
  async function runImportSite() {
    setRunningImport(true)
    try {
      const r = await api.importSite(currentImportOptions())
      setImportPreview(r.import)
      await loadPages()
      await loadSettings()
      message.success(`Imported ${r.import.imported} page(s)`)
    } catch(e:any) {
      message.error(e.message)
    } finally {
      setRunningImport(false)
    }
  }

  useEffect(() => {
    loadSession().catch(e => message.error(e.message))
    loadPages().catch(e => message.error(e.message))
    loadSettings().catch(e => message.error(e.message))
    loadAssets().catch(e => message.error(e.message))
    loadACL().catch(e => message.error(e.message))
    loadThemeHistory().catch(e => message.error(e.message))
  }, [])
  useEffect(() => {
    rememberAdminTheme(adminDark)
  }, [adminDark])
  useEffect(() => {
    for (const [key, value] of Object.entries(adminVars)) {
      document.documentElement.style.setProperty(key, String(value))
    }
  }, [adminVars])

  async function upload(file: File) {
    const data = await readFileData(file)
    return api.uploadFile(file.name, data)
  }
  async function uploadImageForEditor(file: File) {
    const r = await upload(file)
    setAssets(items => [r.asset, ...items.filter(item => item.id !== r.asset.id)])
    return r.asset.url
  }
  function insertAsset(asset: Asset) {
    form.setFieldValue('markdown', `${form.getFieldValue('markdown') || ''}\n\n${assetMarkdown(asset)}\n`)
    setEditorRev(rev => rev + 1)
    setMediaOpen(false)
    message.success('Asset inserted')
  }
  async function deleteAsset(asset: Asset) {
    try {
      const r = await api.deleteAsset(asset.id)
      setAssets(items => items.filter(item => item.id !== asset.id))
      settingsForm.setFieldsValue(r.settings)
      message.success('Media deleted')
    } catch(e:any) {
      message.error(e.message)
    }
  }
  function confirmDeleteAsset(asset: Asset) {
    Modal.confirm({
      title: 'Delete media?',
      content: asset.name,
      okText: 'Delete',
      okButtonProps: { danger: true },
      onOk: () => deleteAsset(asset)
    })
  }

  const contentUploadProps: UploadProps = {
    showUploadList: false,
    beforeUpload(file) {
      upload(file).then(r => {
        setAssets(items => [r.asset, ...items.filter(item => item.id !== r.asset.id)])
        form.setFieldValue('markdown', `${form.getFieldValue('markdown') || ''}\n\n${assetMarkdown(r.asset)}\n`)
        setEditorRev(rev => rev + 1)
        message.success('Uploaded')
      }).catch((e:any) => message.error(e.message))
      return false
    }
  }
  function identityUploadProps(kind: IdentityKind): UploadProps {
    return {
      showUploadList: false,
      accept: '.png,.jpg,.jpeg',
      beforeUpload(file) {
        const ok = /^image\/(png|jpeg)$/.test(file.type) || /\.(png|jpe?g)$/i.test(file.name)
        if (!ok) {
          message.error('Use a PNG or JPG image')
          return false
        }
        setIdentityUploading(kind)
        readFileData(file).then(data => api.setSiteImage(kind, file.name, data)).then(r => {
          setAssets(items => [r.asset, ...items.filter(item => item.id !== r.asset.id)])
          settingsForm.setFieldValue(kind === 'logo' ? 'logo_url' : 'favicon_url', r.asset.url)
          settingsForm.setFieldValue(kind === 'logo' ? 'logo_enabled' : 'favicon_enabled', true)
          message.success(kind === 'logo' ? 'Logo updated' : 'Favicon updated')
        }).catch((e:any) => message.error(e.message)).finally(() => setIdentityUploading(''))
        return false
      }
    }
  }
  function setIdentityFromURL(kind: IdentityKind) {
    const sourceURL = identitySourceURL[kind].trim()
    if (!sourceURL) {
      message.error('Enter an image URL')
      return
    }
    setIdentityUploading(kind)
    api.setSiteImage(kind, '', '', sourceURL).then(r => {
      setAssets(items => [r.asset, ...items.filter(item => item.id !== r.asset.id)])
      settingsForm.setFieldValue(kind === 'logo' ? 'logo_url' : 'favicon_url', r.asset.url)
      settingsForm.setFieldValue(kind === 'logo' ? 'logo_enabled' : 'favicon_enabled', true)
      setIdentitySourceURL(values => ({ ...values, [kind]: '' }))
      message.success(kind === 'logo' ? 'Logo updated from URL' : 'Favicon updated from URL')
    }).catch((e:any) => message.error(e.message)).finally(() => setIdentityUploading(''))
  }
  const mediaUploadProps: UploadProps = {
    showUploadList: false,
    beforeUpload(file) {
      upload(file).then(r => {
        setAssets(items => [r.asset, ...items.filter(item => item.id !== r.asset.id)])
        message.success('Uploaded to media library')
      }).catch((e:any) => message.error(e.message))
      return false
    }
  }

  const mdEditorKey = [active?.slug || 'new', active?.updated_at || '', editorRev].join('-')
  const sessionMenuItems: MenuProps['items'] = [
    { key: 'user', label: <div><Typography.Text type="secondary">Username</Typography.Text><br /><Typography.Text code>{session?.username || 'unknown'}</Typography.Text></div> },
    { key: 'auth', label: <div><Typography.Text type="secondary">Auth</Typography.Text><br /><Typography.Text code>{session?.auth_scheme || 'basic'}</Typography.Text></div> },
    ...(session?.proxy_identity ? Object.entries(session.proxy_identity).map(([key, value]) => ({
      key: `proxy-${key}`,
      label: <div><Typography.Text type="secondary">{key}</Typography.Text><br /><Typography.Text code>{displayValue(value)}</Typography.Text></div>
    })) : []),
    ...(session?.jwt_claims ? [
      { key: 'jwt-source', label: <div><Typography.Text type="secondary">JWT source</Typography.Text><br /><Typography.Text code>{session.jwt_source}</Typography.Text></div> },
      ...Object.entries(session.jwt_claims).map(([key, value]) => ({
        key: `jwt-${key}`,
        label: <div><Typography.Text type="secondary">{key}</Typography.Text><br /><Typography.Text code>{displayValue(value)}</Typography.Text></div>
      }))
    ] : [{ key: 'no-jwt', disabled: true, label: 'No JWT claims on this request' }])
  ]

  return <ConfigProvider theme={cfg} getPopupContainer={trigger => trigger?.parentElement || document.body}><AntApp><Layout className={`layout themeStyle-${themeStyle}`} style={adminVars}>
    <Layout.Sider className="sider" width={310} breakpoint="lg" collapsedWidth={0}>
      <div className="brand">Uvoo-MiniCMS</div>
      <Dropdown menu={{ items: sessionMenuItems }} trigger={['click']}>
        <Button className="accountButton" block>
          {session?.username || 'Signed in'}
        </Button>
      </Dropdown>
      <Space wrap className="palettes">
        {Object.keys(palettes).map(p => <Button key={p} size="small" type={p===palette?'primary':'default'} onClick={() => setPalette(p as Palette)}>{p}</Button>)}
        <Button size="small" type={palette==='custom'?'primary':'default'} onClick={() => { setPalette('custom'); settingsForm.setFieldValue('admin_palette', 'custom') }}>custom</Button>
        <Switch checkedChildren="Dark" unCheckedChildren="Light" checked={adminDark} onChange={checked => setAdminThemeMode(checked ? 'dark' : 'light')} />
      </Space>
      <Space direction="vertical" style={{ width: '100%' }}>
        <Button block type="primary" onClick={() => newPage('page')}>New page</Button>
        <Button block onClick={() => newPage('post')}>New post</Button>
      </Space>
      <List className="pages" dataSource={pages} renderItem={p => <List.Item className={active?.slug===p.slug?'selected':''} onClick={() => openPage(p.slug)}>
        <List.Item.Meta title={p.title} description={`${p.path || `/${p.slug}`}${p.content_type === 'post' ? ' · post' : ''}${p.published ? '' : ' · draft'}`} />
      </List.Item>} />
    </Layout.Sider>
    <Layout.Content className="content">
      <Tabs defaultActiveKey="content" items={[
        { key:'content', label:'Content', children:<Card className="editorCard">
          <Form form={form} layout="vertical" onFinish={savePage} initialValues={freshPage()}>
            <Space className="topbar" align="start">
              <Typography.Title level={3}>{active?.id ? 'Edit page' : 'New page'}</Typography.Title>
              <Space wrap>
                <Switch checkedChildren="Markdown" unCheckedChildren="WYSIWYG" checked={sourceMode} onChange={setSourceMode} />
                {active?.slug && <Button onClick={() => loadRevisions(active.slug)} loading={loadingRevisions}>History</Button>}
                {active?.slug && active.slug !== 'home' && <Popconfirm title="Delete page?" onConfirm={() => removePage(active.slug)}><Button danger>Delete</Button></Popconfirm>}
                <Button href={active?.path || '/'} target="_blank">View</Button>
                <Button type="primary" htmlType="submit" loading={saving}>Save</Button>
              </Space>
            </Space>
            <Form.Item name="title" label="Title" rules={[{required:true}]}><Input onBlur={() => {
              const title = form.getFieldValue('title') || ''
              const nextSlug = slugify(title)
              if (!form.getFieldValue('slug')) form.setFieldValue('slug', nextSlug)
              if (!form.getFieldValue('path')) form.setFieldValue('path', form.getFieldValue('content_type') === 'post' ? joinPath(settingsForm.getFieldValue('blog_path') || '/blog', nextSlug) : pathify(title))
            }} /></Form.Item>
            <Space className="routeGrid" align="start">
              <Form.Item name="slug" label="Admin slug" rules={[{required:true}]}><Input addonBefore="id:" /></Form.Item>
              <Form.Item name="path" label="Public route / SEO URL" rules={[{required:true}]}><Input placeholder="/about/company" /></Form.Item>
              <Form.Item name="content_type" label="Type" rules={[{required:true}]}><Select onChange={updateContentType} options={[{label:'Page', value:'page'}, {label:'Post', value:'post'}]} /></Form.Item>
              <Form.Item name="published" label="Published" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="published_at" label="Published date"><Input placeholder="2026-06-02 or 2026-06-02T12:00:00Z" /></Form.Item>
            </Space>
            <Form.Item name="meta_description" label="SEO description"><Input.TextArea rows={2} maxLength={180} showCount placeholder="Short search/social description for this route." /></Form.Item>
            <Form.Item name="tags" label="Tags"><Input placeholder="news, services, security" /></Form.Item>
            <Space wrap>
              <Upload {...contentUploadProps}><Button>Upload image/file and insert Markdown link</Button></Upload>
              <Button onClick={() => { setMediaOpen(true); loadAssets().catch((e:any) => message.error(e.message)) }}>Browse existing uploads</Button>
            </Space>
            <Form.Item name="markdown" label="Body" className="mdField">
              {sourceMode
                ? <Input.TextArea rows={22} className="sourceEditor" value={md} onChange={e => form.setFieldValue('markdown', e.target.value)} />
                : <React.Suspense fallback={<div className="mdLoading">Loading editor...</div>}><MdBodyEditor editorKey={mdEditorKey} adminDark={adminDark} markdown={md} onChange={v => form.setFieldValue('markdown', v)} uploadImage={uploadImageForEditor} imageSuggestions={imageSuggestions} /></React.Suspense>}
            </Form.Item>
          </Form>
          <Modal title="Browse uploads" open={mediaOpen} onCancel={() => setMediaOpen(false)} footer={null} width={920} className="mediaModal">
            <MediaBrowser assets={assets} loading={loadingAssets} onInsert={insertAsset} onDelete={confirmDeleteAsset} onRefresh={() => loadAssets().catch((e:any) => message.error(e.message))} uploadProps={mediaUploadProps} />
          </Modal>
          <Modal title="Page history" open={historyOpen} onCancel={() => setHistoryOpen(false)} footer={null} width={840}>
            <List
              loading={loadingRevisions}
              dataSource={pageRevisions}
              locale={{ emptyText: 'No saved revisions for this page.' }}
              renderItem={revision => <List.Item actions={[<Button key="restore" onClick={() => restoreRevision(revision)}>Load</Button>]}>
                <List.Item.Meta
                  title={revision.title}
                  description={`${revision.created_at} · ${revision.path} · ${revision.content_type}${revision.published ? '' : ' · draft'}`}
                />
              </List.Item>}
            />
          </Modal>
        </Card> },
        { key:'media', label:'Media', children:<Card className="editorCard">
          <Space className="topbar" align="start">
            <div>
              <Typography.Title level={3}>Media library</Typography.Title>
              <Typography.Text type="secondary">Browse uploaded files and insert reusable images into the active page.</Typography.Text>
            </div>
            <Space wrap>
              <Upload {...mediaUploadProps}><Button type="primary">Upload media</Button></Upload>
              <Button onClick={() => loadAssets().catch((e:any) => message.error(e.message))} loading={loadingAssets}>Refresh</Button>
            </Space>
          </Space>
          <MediaBrowser assets={assets} loading={loadingAssets} onInsert={insertAsset} onDelete={confirmDeleteAsset} onRefresh={() => loadAssets().catch((e:any) => message.error(e.message))} uploadProps={mediaUploadProps} />
        </Card> },
        { key:'site', label:'Site', children:<Card className="editorCard">
          <Form form={settingsForm} layout="vertical" onFinish={() => saveSettings()} initialValues={{site_name:'Uvoo-MiniCMS', default_theme:'dark', public_theme_style:'soft', public_primary_color:'#386bc0', public_secondary_color:'#64748b', public_header_style:'neutral', admin_theme:'dark', theme_style:'soft', admin_primary_color:'#386bc0', admin_secondary_color:'#64748b', admin_palette:'slate', nav_layout:'top', footer_markdown:'', logo_enabled:true, favicon_enabled:true, menu_enabled:true, footer_enabled:true, theme_toggle_enabled:true, icons_enabled:true, search_enabled:true, blog_enabled:false, blog_path:'/blog', blog_title:'Blog', blog_menu_enabled:true, blog_posts_per_page:20, revision_history_limit:0, menu:[{id:'home', type:'link', parent_id:'', label:'Home', url:'/', external:false, enabled:true}]}}>
            <Space className="topbar" align="start">
              <div>
                <Typography.Title level={3}>Site settings</Typography.Title>
                <Typography.Text type="secondary">Logo, favicon, top menu, and public light/dark default.</Typography.Text>
              </div>
              <Button type="primary" htmlType="submit" loading={savingSettings}>Save site</Button>
            </Space>
            <Space className="switchGrid" wrap>
              <Form.Item name="logo_enabled" label="Logo" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="favicon_enabled" label="Favicon" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="menu_enabled" label="Top menu" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="theme_toggle_enabled" label="Guest theme toggle" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="icons_enabled" label="Font Awesome icons" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="search_enabled" label="Search" valuePropName="checked"><Switch /></Form.Item>
            </Space>
            <Form.Item name="site_name" label="Site name" rules={[{required:true}]}><Input /></Form.Item>
            <div className="identityGrid">
              <div className="identityPanel">
                <div className="identityPreview logoIdentityPreview">{logoURL ? <img src={logoURL} alt="" /> : <span>Logo</span>}</div>
                <Form.Item name="logo_url" label="Current logo URL"><Input placeholder="/uploads/... or https://..." /></Form.Item>
                <Input.Group compact className="identitySource">
                  <Input value={identitySourceURL.logo} onChange={e => setIdentitySourceURL(values => ({ ...values, logo: e.target.value }))} placeholder="PNG/JPG URL to optimize" />
                  <Button onClick={() => setIdentityFromURL('logo')} loading={identityUploading === 'logo'}>Set from URL</Button>
                </Input.Group>
                <Space wrap>
                  <Upload {...identityUploadProps('logo')}><Button loading={identityUploading === 'logo'}>Set logo from PNG/JPG</Button></Upload>
                  <Button onClick={() => settingsForm.setFieldValue('logo_url', '')}>Clear</Button>
                </Space>
              </div>
              <div className="identityPanel">
                <div className="identityPreview faviconIdentityPreview">{faviconURL ? <img src={faviconURL} alt="" /> : <span>Icon</span>}</div>
                <Form.Item name="favicon_url" label="Current favicon URL"><Input placeholder="/uploads/... or https://..." /></Form.Item>
                <Input.Group compact className="identitySource">
                  <Input value={identitySourceURL.favicon} onChange={e => setIdentitySourceURL(values => ({ ...values, favicon: e.target.value }))} placeholder="PNG/JPG URL to optimize" />
                  <Button onClick={() => setIdentityFromURL('favicon')} loading={identityUploading === 'favicon'}>Set from URL</Button>
                </Input.Group>
                <Space wrap>
                  <Upload {...identityUploadProps('favicon')}><Button loading={identityUploading === 'favicon'}>Set favicon from PNG/JPG</Button></Upload>
                  <Button onClick={() => settingsForm.setFieldValue('favicon_url', '')}>Clear</Button>
                </Space>
              </div>
            </div>
            <Form.Item name="default_theme" label="Public default theme"><Select onChange={value => setPublicTheme(value)} options={[{label:'Light', value:'light'}, {label:'Dark', value:'dark'}]} /></Form.Item>
            <Form.Item name="nav_layout" label="Public navigation layout"><Select options={[{label:'Top menu', value:'top'}, {label:'Side drawer', value:'side'}]} /></Form.Item>
            <Typography.Title level={4}>Blog</Typography.Title>
            <Space className="switchGrid" wrap>
              <Form.Item name="blog_enabled" label="Blog route" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="blog_menu_enabled" label="Add Blog menu item" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="blog_posts_per_page" label="Posts per page"><Input type="number" min={1} max={100} /></Form.Item>
              <Form.Item name="revision_history_limit" label="Revisions per page"><Input type="number" min={0} max={100} /></Form.Item>
            </Space>
            <Space className="routeGrid" align="start">
              <Form.Item name="blog_title" label="Blog menu/title"><Input placeholder="Blog" /></Form.Item>
              <Form.Item name="blog_path" label="Blog route"><Input placeholder="/blog" /></Form.Item>
            </Space>
            <Typography.Title level={4}>Top menu</Typography.Title>
            <Form.List name="menu">{(fields, { add, remove }) => <>
              <Space className="menuToolbar" wrap>
                <Input allowClear placeholder="Search menu items" value={menuSearch} onChange={e => setMenuSearch(e.target.value)} />
                <Button onClick={sortCurrentMenuAlphabetically}>Sort A-Z</Button>
                <Button onClick={() => add({id:newID(), type:'link', parent_id:'', label:'', url:'/', external:false, enabled:true})}>Add menu item</Button>
              </Space>
              {fields.filter(field => !menuSearchQuery || menuSearchValue(menuItems?.[field.name]).includes(menuSearchQuery)).map(field => <Space key={field.key} className="menuRow" align="start">
                <Form.Item {...field} name={[field.name, 'id']} hidden><Input /></Form.Item>
                <Form.Item {...field} name={[field.name, 'type']} label="Type"><Select options={[{label:'Link', value:'link'}, {label:'Section', value:'section'}]} /></Form.Item>
                <div>
                  <Form.Item {...field} name={[field.name, 'label']} label="Label" rules={[{required:true}]}><Input placeholder="About" /></Form.Item>
                  {menuItems?.[field.name]?.type === 'section' && !(menuItems || []).some((item, i) => i !== field.name && item?.parent_id === menuItems[field.name]?.id) && <Typography.Text type="warning">Section has no child items.</Typography.Text>}
                </div>
                <Form.Item noStyle shouldUpdate={(prev, cur) => prev.menu?.[field.name]?.type !== cur.menu?.[field.name]?.type}>
                  {({ getFieldValue }) => {
                    const itemType = getFieldValue(['menu', field.name, 'type']) === 'section' ? 'section' : 'link'
                    return <Form.Item {...field} name={[field.name, 'url']} label="URL" rules={itemType === 'section' ? [] : [{required:true}]}>
                      <Input disabled={itemType === 'section'} placeholder={itemType === 'section' ? 'Not used for sections' : '/about or https://...'} />
                    </Form.Item>
                  }}
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'parent_id']} label="Parent"><Select allowClear placeholder="Top level" options={menuParentOptions(menuItems || [], field.name)} /></Form.Item>
                <Form.Item noStyle shouldUpdate={(prev, cur) => prev.menu?.[field.name]?.type !== cur.menu?.[field.name]?.type}>
                  {({ getFieldValue }) => <Form.Item {...field} name={[field.name, 'external']} label="External" valuePropName="checked"><Switch disabled={getFieldValue(['menu', field.name, 'type']) === 'section'} /></Form.Item>}
                </Form.Item>
                <Form.Item {...field} name={[field.name, 'enabled']} label="Enabled" valuePropName="checked"><Switch /></Form.Item>
                <div className="menuOrderActions">
                  <Typography.Text type="secondary">Order</Typography.Text>
                  <Space.Compact>
                    <Button aria-label="Move menu item up" disabled={siblingMenuIndex(menuItems || [], field.name, -1) < 0} onClick={() => moveMenuItem(field.name, -1)}>Up</Button>
                    <Button aria-label="Move menu item down" disabled={siblingMenuIndex(menuItems || [], field.name, 1) < 0} onClick={() => moveMenuItem(field.name, 1)}>Down</Button>
                  </Space.Compact>
                  <Button danger onClick={() => remove(field.name)}>Remove</Button>
                </div>
              </Space>)}
              {menuSearchQuery && fields.every(field => !menuSearchValue(menuItems?.[field.name]).includes(menuSearchQuery)) && <Typography.Text type="secondary">No menu items match this search.</Typography.Text>}
            </>}</Form.List>
          </Form>
        </Card> },
        { key:'footer', label:'Footer', children:<Card className="editorCard">
          <Space className="topbar" align="start">
            <div>
              <Typography.Title level={3}>Footer</Typography.Title>
              <Typography.Text type="secondary">Global Markdown shown at the bottom of public pages.</Typography.Text>
            </div>
            <Button type="primary" loading={savingSettings} onClick={() => saveSettings({ footer_enabled: footerEnabled, footer_markdown: footerMarkdown })}>Save footer</Button>
          </Space>
          <Space className="switchGrid" wrap>
            <label className="footerToggle">
              <Typography.Text strong>Footer enabled</Typography.Text>
              <Switch checked={footerEnabled} onChange={setFooterEnabled} />
            </label>
          </Space>
          <Input.TextArea className="sourceEditor footerEditor" rows={12} value={footerMarkdown} onChange={e => setFooterMarkdown(e.target.value)} placeholder={defaultFooter(settingsForm.getFieldValue('site_name') || 'Your Company')} />
          <Typography.Paragraph type="secondary" className="footerHint">
            Markdown supports contact lines, address, internal links, external links, and social profiles.
          </Typography.Paragraph>
        </Card> },
        { key:'import', label:'Import', children:<Card className="editorCard">
          <Space className="topbar" align="start">
            <div>
              <Typography.Title level={3}>Import website</Typography.Title>
              <Typography.Text type="secondary">Pull pages and menu items from WordPress REST, XML sitemaps, or same-site links.</Typography.Text>
            </div>
            <Space wrap>
              <Button onClick={previewImportSite} loading={previewingImport}>Preview</Button>
              <Button type="primary" onClick={runImportSite} loading={runningImport} disabled={!importPreview?.pages?.length}>Import</Button>
            </Space>
          </Space>
          <Form layout="vertical" className="importForm">
            <Form.Item label="Website URL" required>
              <Input value={importURL} onChange={e => setImportURL(e.target.value)} placeholder="https://example.com/" />
            </Form.Item>
            <Space className="switchGrid" wrap>
              <Form.Item label="Max pages">
                <Input type="number" min={1} max={200} value={importMaxPages} onChange={e => setImportMaxPages(Math.max(1, Math.min(200, Number(e.target.value) || 50)))} />
              </Form.Item>
              <Form.Item label="WordPress posts" valuePropName="checked"><Switch checked={importIncludePosts} onChange={setImportIncludePosts} /></Form.Item>
              <Form.Item label="Menu" valuePropName="checked"><Switch checked={importMenu} onChange={setImportMenu} /></Form.Item>
              <Form.Item label="Publish" valuePropName="checked"><Switch checked={importPublish} onChange={setImportPublish} /></Form.Item>
              <Form.Item label="Update existing" valuePropName="checked"><Switch checked={importUpdateExisting} onChange={setImportUpdateExisting} /></Form.Item>
              <Form.Item label="Download images" valuePropName="checked"><Switch checked={importDownloadImages} onChange={setImportDownloadImages} /></Form.Item>
              <Form.Item label="Advanced scraping" valuePropName="checked"><Switch checked={importAdvancedScraping} onChange={setImportAdvancedScraping} /></Form.Item>
            </Space>
          </Form>
          {importPreview && <div className="importPreview">
            <Space wrap className="importSummary">
              <Typography.Text strong>{importPreview.wordpress ? 'WordPress' : importPreview.source || 'Website'} detected</Typography.Text>
              <Typography.Text type="secondary">{importPreview.pages.length} page(s)</Typography.Text>
              <Typography.Text type="secondary">{importPreview.menu.length} menu item(s)</Typography.Text>
              {importPreview.existing > 0 && <Typography.Text type="warning">{importPreview.existing} existing</Typography.Text>}
              {importPreview.imported > 0 && <Typography.Text type="success">{importPreview.imported} imported</Typography.Text>}
              {importPreview.skipped > 0 && <Typography.Text type="secondary">{importPreview.skipped} skipped</Typography.Text>}
            </Space>
            {importPreview.errors.length > 0 && <List className="importErrors" dataSource={importPreview.errors} renderItem={err => <List.Item><Typography.Text type="danger">{err}</Typography.Text></List.Item>} />}
            <Typography.Title level={4}>Pages</Typography.Title>
            <List className="importList" dataSource={importPreview.pages} renderItem={page => <List.Item>
              <List.Item.Meta
                title={<Space wrap><span>{page.title}</span>{page.exists && <Typography.Text type="warning">existing</Typography.Text>}</Space>}
                description={`${page.path} · ${page.content_type} · ${page.source_url}`}
              />
            </List.Item>} />
            {importPreview.menu.length > 0 && <>
              <Typography.Title level={4}>Menu</Typography.Title>
              <List className="importList" dataSource={importPreview.menu} renderItem={item => <List.Item>
                <List.Item.Meta title={item.label} description={`${item.url}${item.parent_id ? ` · child of ${item.parent_id}` : ''}`} />
              </List.Item>} />
            </>}
          </div>}
        </Card> },
        { key:'security', label:'Security', children:<Card className="editorCard">
          <Space className="topbar" align="start">
            <div>
              <Typography.Title level={3}>Security</Typography.Title>
              <Typography.Text type="secondary">Small SQLite-backed network rules for admin/API and public traffic. Environment CIDR rules still run first.</Typography.Text>
            </div>
            <Button type="primary" loading={savingACL} onClick={saveACL}>Save security</Button>
          </Space>
          <Typography.Paragraph type="secondary" className="securityNote">
            Use CIDR rules like <code>203.0.113.10/32</code>, <code>198.51.100.0/24</code>, or <code>2001:db8::/32</code>. Country rules require <code>CMS_MAXMIND_DB</code>; leave allow lists empty unless you want to restrict to only those countries.
          </Typography.Paragraph>
          <div className="securityGrid">
            <label>
              <Typography.Text strong>Admin/API default</Typography.Text>
              <Select value={acl.admin_default} onChange={value => setACLField('admin_default', value)} options={[{label:'Allow unless denied', value:'allow'}, {label:'Deny unless allowed', value:'deny'}]} />
            </label>
            <label>
              <Typography.Text strong>Public/uploads default</Typography.Text>
              <Select value={acl.public_default} onChange={value => setACLField('public_default', value)} options={[{label:'Allow unless denied', value:'allow'}, {label:'Deny unless allowed', value:'deny'}]} />
            </label>
            <label>
              <Typography.Text strong>Admin country allow</Typography.Text>
              <Input value={acl.admin_allow_countries} onChange={e => setACLField('admin_allow_countries', e.target.value)} placeholder="US, CA" />
            </label>
            <label>
              <Typography.Text strong>Admin country deny</Typography.Text>
              <Input value={acl.admin_deny_countries} onChange={e => setACLField('admin_deny_countries', e.target.value)} placeholder="RU, KP" />
            </label>
            <label>
              <Typography.Text strong>Public country allow</Typography.Text>
              <Input value={acl.public_allow_countries} onChange={e => setACLField('public_allow_countries', e.target.value)} placeholder="US, CA, GB" />
            </label>
            <label>
              <Typography.Text strong>Public country deny</Typography.Text>
              <Input value={acl.public_deny_countries} onChange={e => setACLField('public_deny_countries', e.target.value)} placeholder="RU, KP" />
            </label>
          </div>
          <Space className="aclHeader" align="center">
            <Typography.Title level={4}>CIDR rules</Typography.Title>
            <Button onClick={addACLRule}>Add rule</Button>
          </Space>
          {acl.rules.length === 0 && <div className="emptyMedia"><Typography.Text type="secondary">No database ACL rules yet. Defaults and environment rules still apply.</Typography.Text></div>}
          {acl.rules.map((rule, index) => <div className="aclRow" key={`${rule.id || 'new'}-${index}`}>
            <label>
              <Typography.Text>Scope</Typography.Text>
              <Select value={rule.scope} onChange={value => updateACLRule(index, { scope: value })} options={[{label:'All', value:'all'}, {label:'Admin/API', value:'admin'}, {label:'Public', value:'public'}]} />
            </label>
            <label>
              <Typography.Text>Action</Typography.Text>
              <Select value={rule.action} onChange={value => updateACLRule(index, { action: value })} options={[{label:'Allow', value:'allow'}, {label:'Deny', value:'deny'}]} />
            </label>
            <label>
              <Typography.Text>CIDR</Typography.Text>
              <Input value={rule.cidr} onChange={e => updateACLRule(index, { cidr: e.target.value })} placeholder="203.0.113.10/32" />
            </label>
            <label className="aclSwitch">
              <Typography.Text>Enabled</Typography.Text>
              <Switch checked={rule.enabled !== false} onChange={checked => updateACLRule(index, { enabled: checked })} />
            </label>
            <label>
              <Typography.Text>Note</Typography.Text>
              <Input value={rule.note} onChange={e => updateACLRule(index, { note: e.target.value })} placeholder="office, VPN, vendor" />
            </label>
            <Button danger onClick={() => removeACLRule(index)}>Remove</Button>
          </div>)}
          <Space wrap className="securityActions">
            <Button onClick={addACLRule}>Add rule</Button>
            <Button type="primary" loading={savingACL} onClick={saveACL}>Save security</Button>
          </Space>
        </Card> },
        { key:'theme', label:'Theme', children:<Card className="editorCard">
          <Space className="topbar" align="start">
            <div>
              <Typography.Title level={3}>Theme</Typography.Title>
              <Typography.Text type="secondary">Set the saved admin and public themes independently, with the same light/dark and color controls for both.</Typography.Text>
            </div>
          </Space>
          <Space wrap className="themePicker">
            {Object.keys(palettes).map(p => <Button key={p} type={p===palette?'primary':'default'} onClick={() => { setPalette(p as Palette); settingsForm.setFieldValue('admin_palette', p) }}>{p}</Button>)}
            <Button type={palette==='custom'?'primary':'default'} onClick={() => { setPalette('custom'); settingsForm.setFieldValue('admin_palette', 'custom') }}>Custom</Button>
            <Switch checkedChildren="Dark" unCheckedChildren="Light" checked={adminDark} onChange={checked => setAdminThemeMode(checked ? 'dark' : 'light')} />
          </Space>
          {themeHistory.length > 0 && <div className="themeHistory">
            <Typography.Text strong>Recent themes</Typography.Text>
            <Select className="recentThemeSelect" placeholder="Apply recent theme" onChange={id => {
              const theme = themeHistory.find(item => item.id === id)
              if (theme) applyThemeHistory(theme)
            }} options={themeHistory.map(theme => ({ label: themeLabel(theme), value: theme.id }))} />
            <Button onClick={() => loadThemeHistory().catch((e:any) => message.error(e.message))}>Refresh</Button>
          </div>}
          <Form layout="vertical" className="customThemeForm">
            <Typography.Title level={4}>Admin theme</Typography.Title>
            <div className="themeControlGrid">
              <Form.Item label="Default mode">
                <Select className="themeSelect" value={adminDark ? 'dark' : 'light'} onChange={(value: 'light'|'dark') => setAdminThemeMode(value)} options={[{label:'Light', value:'light'}, {label:'Dark', value:'dark'}]} />
              </Form.Item>
              <Form.Item label="UI style">
                <Select className="themeSelect" value={themeStyle} onChange={value => { setThemeStyle(value); settingsForm.setFieldValue('theme_style', value) }} options={themeStyleOptions} />
              </Form.Item>
            </div>
            <Form.Item label="Custom primary color">
              <Space>
                <Input type="color" value={customPrimary} onChange={e => { setCustomPrimary(e.target.value); setPalette('custom'); settingsForm.setFieldValue('admin_primary_color', e.target.value); settingsForm.setFieldValue('admin_palette', 'custom') }} className="colorInput" />
                <Input value={customPrimary} onChange={e => { const value = e.target.value.startsWith('#') ? e.target.value : `#${e.target.value}`; setCustomPrimary(value); setPalette('custom'); settingsForm.setFieldValue('admin_primary_color', value); settingsForm.setFieldValue('admin_palette', 'custom') }} />
              </Space>
            </Form.Item>
            <Form.Item label="Custom secondary color">
              <Space>
                <Input type="color" value={customSecondary} onChange={e => { setCustomSecondary(e.target.value); settingsForm.setFieldValue('admin_secondary_color', e.target.value) }} className="colorInput" />
                <Input value={customSecondary} onChange={e => { const value = e.target.value.startsWith('#') ? e.target.value : `#${e.target.value}`; setCustomSecondary(value); settingsForm.setFieldValue('admin_secondary_color', value) }} />
              </Space>
            </Form.Item>
            <Button type="primary" loading={savingSettings} onClick={() => saveSettings()}>Save admin theme</Button>
            <Typography.Paragraph type="secondary">Suggested primary: <code>#386bc0</code>. The admin background, buttons, inputs, tabs, editor, sliders, and selected states derive from the active theme.</Typography.Paragraph>
          </Form>
          <div className="publicThemePanel">
            <Typography.Title level={4}>Public theme</Typography.Title>
            <Typography.Paragraph type="secondary">These settings are saved and applied to visitor-facing pages.</Typography.Paragraph>
            <Space wrap align="end">
              <div>
                <Typography.Text>Default mode</Typography.Text>
                <Select className="themeSelect" value={publicTheme} onChange={value => { setPublicTheme(value); settingsForm.setFieldValue('default_theme', value) }} options={[{label:'Light', value:'light'}, {label:'Dark', value:'dark'}]} />
              </div>
              <div>
                <Typography.Text>UI style</Typography.Text>
                <Select className="themeSelect" value={publicThemeStyle} onChange={value => { setPublicThemeStyle(value); settingsForm.setFieldValue('public_theme_style', value) }} options={themeStyleOptions} />
              </div>
              <div>
                <Typography.Text>Primary color</Typography.Text>
                <Space>
                  <Input type="color" value={publicPrimary} onChange={e => { setPublicPrimary(e.target.value); settingsForm.setFieldValue('public_primary_color', e.target.value) }} className="colorInput" />
                  <Input value={publicPrimary} onChange={e => { const value = e.target.value.startsWith('#') ? e.target.value : `#${e.target.value}`; setPublicPrimary(value); settingsForm.setFieldValue('public_primary_color', value) }} />
                </Space>
              </div>
              <div>
                <Typography.Text>Secondary color</Typography.Text>
                <Space>
                  <Input type="color" value={publicSecondary} onChange={e => { setPublicSecondary(e.target.value); settingsForm.setFieldValue('public_secondary_color', e.target.value) }} className="colorInput" />
                  <Input value={publicSecondary} onChange={e => { const value = e.target.value.startsWith('#') ? e.target.value : `#${e.target.value}`; setPublicSecondary(value); settingsForm.setFieldValue('public_secondary_color', value) }} />
                </Space>
              </div>
              <div>
                <Typography.Text>Header style</Typography.Text>
                <Select className="themeSelect" value={publicHeaderStyle} onChange={value => { setPublicHeaderStyle(value); settingsForm.setFieldValue('public_header_style', value) }} options={[
                  {label:'Neutral', value:'neutral'},
                  {label:'Accent line', value:'accent-line'},
                  {label:'Accent background', value:'accent-bg'}
                ]} />
              </div>
              <Button onClick={() => { setPublicPrimary(adminTokens.colorPrimary); settingsForm.setFieldValue('public_primary_color', adminTokens.colorPrimary) }}>Use admin primary</Button>
              <Button type="primary" loading={savingSettings} onClick={() => saveSettings({ default_theme: publicTheme, public_theme_style: publicThemeStyle, public_primary_color: publicPrimary, public_secondary_color: publicSecondary, public_header_style: publicHeaderStyle })}>Save public theme</Button>
            </Space>
          </div>
        </Card> }
      ]} />
    </Layout.Content>
  </Layout></AntApp></ConfigProvider>
}

function MediaBrowser({ assets, loading, onInsert, onDelete, onRefresh, uploadProps }: { assets: Asset[]; loading: boolean; onInsert: (asset: Asset) => void; onDelete: (asset: Asset) => void; onRefresh: () => void; uploadProps: UploadProps }) {
  const images = assets.filter(asset => isImage(asset.url))
  const files = assets.filter(asset => !isImage(asset.url))
  const assetMenu = (asset: Asset): MenuProps => ({
    items: [
      { key: 'insert', label: isImage(asset.url) ? 'Insert' : 'Insert link' },
      { key: 'view', label: 'View' },
      { key: 'delete', label: 'Delete', danger: true }
    ],
    onClick: ({ key }) => {
      if (key === 'insert') onInsert(asset)
      if (key === 'view') window.open(asset.url, '_blank', 'noopener,noreferrer')
      if (key === 'delete') onDelete(asset)
    }
  })
  const actionButton = (asset: Asset) => (
    <Dropdown trigger={['click']} menu={assetMenu(asset)}>
      <Button size="small" aria-label={`Actions for ${asset.name}`}>...</Button>
    </Dropdown>
  )
  return <div>
    <Space wrap className="mediaActions">
      <Upload {...uploadProps}><Button>Upload file</Button></Upload>
      <Button onClick={onRefresh} loading={loading}>Refresh</Button>
      <Typography.Text type="secondary">{assets.length} upload(s)</Typography.Text>
    </Space>
    <Tabs items={[
      { key:'images', label:`Images (${images.length})`, children: images.length ? <div className="mediaGrid">
        {images.map(asset => <div className="mediaTile" key={asset.id}>
          <button type="button" className="mediaPreview" onClick={() => onInsert(asset)} aria-label={`Insert ${asset.name}`}>
            <img src={asset.url} alt={asset.name} />
          </button>
          <Typography.Text ellipsis title={asset.name}>{asset.name}</Typography.Text>
          <div className="mediaTileActions">{actionButton(asset)}</div>
        </div>)}
      </div> : <div className="emptyMedia"><Typography.Text type="secondary">No uploaded images yet.</Typography.Text></div> },
      { key:'files', label:`Files (${files.length})`, children: files.length ? <List className="mediaFileList" dataSource={files} renderItem={asset => <List.Item actions={[actionButton(asset)]}>
        <List.Item.Meta title={asset.name} description={`${Math.round(asset.size / 1024)} KB · ${asset.url}`} />
      </List.Item>} /> : <div className="emptyMedia"><Typography.Text type="secondary">No uploaded files yet.</Typography.Text></div> }
    ]} />
  </div>
}

createRoot(document.getElementById('root')!).render(<Root />)
