"use strict"

import { syncSidebarSearchTypeFromPane, setActivePanelInputs } from './search-filter-utils.js'

const SIDEBAR_DOC_PANEL  = 'search-sidebar-documents-panel'
const SIDEBAR_AUTH_PANEL = 'search-sidebar-authors-panel'
const SIDEBAR_TYPE_INPUT = 'search-type'

function setOffcanvasSearchType(type) {
    const typeInput = document.getElementById("search-offcanvas-type")
    if (typeInput) typeInput.value = type
}

function syncOffcanvasSearchTypeFromPane() {
    const authorPane = document.getElementById("search-offcanvas-authors-panel")
    const type = authorPane?.classList.contains("active") ? "authors" : "documents"
    setOffcanvasSearchType(type)
    return type
}

function setOffcanvasActivePanelInputs(activeType) {
    const docPanel    = document.getElementById("search-offcanvas-documents-panel")
    const authorPanel = document.getElementById("search-offcanvas-authors-panel")
    docPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        if (el.id === "search-offcanvas-type") return
        el.disabled = activeType !== "documents"
    })
    authorPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        el.disabled = activeType !== "authors"
    })
}

function switchSidebarPanes(tabName) {
    const sidebarTabId = tabName === "authors" ? "search-sidebar-tab-authors" : "search-sidebar-tab-documents"
    const sidebarTab = document.getElementById(sidebarTabId)
    if (sidebarTab && typeof bootstrap !== "undefined") {
        bootstrap.Tab.getOrCreateInstance(sidebarTab).show()
    }
    setActivePanelInputs(tabName, SIDEBAR_DOC_PANEL, SIDEBAR_AUTH_PANEL, SIDEBAR_TYPE_INPUT)
}

function switchOffcanvasPanes(tabName) {
    const docPane       = document.getElementById("search-offcanvas-documents-panel")
    const authorPane    = document.getElementById("search-offcanvas-authors-panel")
    const offcanvasTabs = document.getElementById("search-offcanvas-tabs")
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

    setActivePanelInputs(tabName, 'search-offcanvas-documents-panel', 'search-offcanvas-authors-panel', 'search-offcanvas-type')
}

function applyFilters() {
    const form = document.getElementById("search-filters-form")
    if (form?._coreanderApplyFilters) {
        form._coreanderApplyFilters()
    } else if (window.htmx) {
        window.htmx.trigger(document.body, "update")
    }
}

const offcanvasTabs = document.getElementById("search-offcanvas-tabs")

if (offcanvasTabs) {
    const activeType = syncOffcanvasSearchTypeFromPane()
    setOffcanvasActivePanelInputs(activeType)
}

if (document.getElementById("search-filters-form")) {
    const initialType = syncSidebarSearchTypeFromPane()
    setActivePanelInputs(initialType, SIDEBAR_DOC_PANEL, SIDEBAR_AUTH_PANEL, SIDEBAR_TYPE_INPUT)

    document.body.addEventListener('htmx:configRequest', function (evt) {
        const sidebarForm = document.getElementById('search-filters-form')
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
        document.querySelectorAll('.placeholder-skeleton-doc').forEach(el => el.classList.toggle('d-none', tabName === 'authors'))
        document.querySelectorAll('.placeholder-skeleton-author').forEach(el => el.classList.toggle('d-none', tabName !== 'authors'))
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
        syncOffcanvasSearchTypeFromPane()
    })
}
