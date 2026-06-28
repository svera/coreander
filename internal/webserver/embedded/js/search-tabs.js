"use strict"

import { syncSidebarSearchTypeFromPane, syncSearchTypeFromPane, setActivePanelInputs } from './search-filter-utils.js'

const SIDEBAR_DOC_PANEL      = 'search-sidebar-documents-panel'
const SIDEBAR_AUTH_PANEL     = 'search-sidebar-authors-panel'
const SIDEBAR_TYPE_INPUT     = 'search-type'
const SIDEBAR_TAB_DOCS_ID    = 'search-sidebar-tab-documents'
const SIDEBAR_TAB_AUTHORS_ID = 'search-sidebar-tab-authors'
const OFFCANVAS_DOC_PANEL    = 'search-offcanvas-documents-panel'
const OFFCANVAS_AUTH_PANEL   = 'search-offcanvas-authors-panel'
const OFFCANVAS_TYPE_INPUT   = 'search-offcanvas-type'
const OFFCANVAS_TABS_ID      = 'search-offcanvas-tabs'
const SIDEBAR_FORM_ID        = 'search-filters-form'
const SKELETON_DOC_CLASS     = '.placeholder-skeleton-doc'
const SKELETON_AUTH_CLASS    = '.placeholder-skeleton-author'

function switchSidebarPanes(tabName) {
    const sidebarTabId = tabName === "authors" ? SIDEBAR_TAB_AUTHORS_ID : SIDEBAR_TAB_DOCS_ID
    const sidebarTab = document.getElementById(sidebarTabId)
    if (sidebarTab && typeof bootstrap !== "undefined") {
        bootstrap.Tab.getOrCreateInstance(sidebarTab).show()
    }
    setActivePanelInputs(tabName, SIDEBAR_DOC_PANEL, SIDEBAR_AUTH_PANEL, SIDEBAR_TYPE_INPUT)
}

function switchOffcanvasPanes(tabName) {
    const docPane       = document.getElementById(OFFCANVAS_DOC_PANEL)
    const authorPane    = document.getElementById(OFFCANVAS_AUTH_PANEL)
    const offcanvasTabs = document.getElementById(OFFCANVAS_TABS_ID)
    const docBtn        = offcanvasTabs?.querySelector("[data-search-type-tab='documents']")
    const authorBtn     = offcanvasTabs?.querySelector("[data-search-type-tab='authors']")

    const toDoc = tabName === "documents"
    docPane?.classList.toggle("show", toDoc)
    docPane?.classList.toggle("active", toDoc)
    authorPane?.classList.toggle("show", !toDoc)
    authorPane?.classList.toggle("active", !toDoc)
    docBtn?.classList.toggle("active", toDoc)
    docBtn?.setAttribute("aria-selected", String(toDoc))
    authorBtn?.classList.toggle("active", !toDoc)
    authorBtn?.setAttribute("aria-selected", String(!toDoc))

    setActivePanelInputs(tabName, OFFCANVAS_DOC_PANEL, OFFCANVAS_AUTH_PANEL, OFFCANVAS_TYPE_INPUT)
}

function applyFilters() {
    const form = document.getElementById(SIDEBAR_FORM_ID)
    if (form?._coreanderApplyFilters) {
        form._coreanderApplyFilters()
    } else if (window.htmx) {
        window.htmx.trigger(document.body, "update")
    }
}

const offcanvasTabs = document.getElementById(OFFCANVAS_TABS_ID)

if (offcanvasTabs) {
    const activeType = syncSearchTypeFromPane(OFFCANVAS_TYPE_INPUT, OFFCANVAS_AUTH_PANEL)
    setActivePanelInputs(activeType, OFFCANVAS_DOC_PANEL, OFFCANVAS_AUTH_PANEL, OFFCANVAS_TYPE_INPUT)
}

if (document.getElementById(SIDEBAR_FORM_ID)) {
    const initialType = syncSidebarSearchTypeFromPane()
    setActivePanelInputs(initialType, SIDEBAR_DOC_PANEL, SIDEBAR_AUTH_PANEL, SIDEBAR_TYPE_INPUT)

    document.body.addEventListener('htmx:configRequest', function (evt) {
        const sidebarForm = document.getElementById(SIDEBAR_FORM_ID)
        if (!sidebarForm) return
        for (const el of sidebarForm.elements) {
            if (!el.name || !el.disabled) continue
            if (el.type === 'submit' || el.type === 'button' || el.type === 'reset' || el.type === 'image') continue
            if ((el.type === 'checkbox' || el.type === 'radio') && !el.checked) continue
            const v = el.value
            if (v == null || String(v).trim() === '') continue
            if (!(el.name in evt.detail.parameters)) {
                evt.detail.parameters[el.name] = v
            }
        }
    })

    document.addEventListener("click", (event) => {
        const tabBtn = event.target?.closest("[data-search-type-tab]")
        if (!tabBtn) return
        const tabName = tabBtn.dataset.searchTypeTab
        if (tabName !== "documents" && tabName !== "authors") return

        switchSidebarPanes(tabName)
        switchOffcanvasPanes(tabName)
        document.querySelectorAll(SKELETON_DOC_CLASS).forEach(el => el.classList.toggle('d-none', tabName === 'authors'))
        document.querySelectorAll(SKELETON_AUTH_CLASS).forEach(el => el.classList.toggle('d-none', tabName !== 'authors'))
        applyFilters()
    })

} else if (offcanvasTabs) {
    offcanvasTabs.addEventListener("click", (event) => {
        const tabBtn = event.target?.closest("[data-search-type-tab]")
        if (!tabBtn) return
        const tabName = tabBtn.dataset.searchTypeTab
        if (tabName !== "authors" && tabName !== "documents") return
        switchOffcanvasPanes(tabName)
    })

    offcanvasTabs.closest("form")?.addEventListener("submit", () => {
        syncSearchTypeFromPane(OFFCANVAS_TYPE_INPUT, OFFCANVAS_AUTH_PANEL)
    })
}
