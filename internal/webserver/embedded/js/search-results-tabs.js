"use strict"

import { syncSidebarSearchTypeFromPane, setActivePanelInputs } from './search-filter-utils.js'

const SIDEBAR_DOC_PANEL  = 'search-sidebar-documents-panel'
const SIDEBAR_AUTH_PANEL = 'search-sidebar-authors-panel'
const SIDEBAR_TYPE_INPUT = 'search-type'

function switchSidebarPanes(tabName) {
    const sidebarTabId = tabName === "authors" ? "search-sidebar-tab-authors" : "search-sidebar-tab-documents"
    const sidebarTab = document.getElementById(sidebarTabId)
    if (sidebarTab && typeof bootstrap !== "undefined") {
        bootstrap.Tab.getOrCreateInstance(sidebarTab).show()
    }
    setActivePanelInputs(tabName, SIDEBAR_DOC_PANEL, SIDEBAR_AUTH_PANEL, SIDEBAR_TYPE_INPUT)
}

function switchOffcanvasPanes(tabName) {
    const docPane    = document.getElementById("search-offcanvas-documents-panel")
    const authorPane = document.getElementById("search-offcanvas-authors-panel")
    const docBtn     = document.querySelector("#search-offcanvas-tabs [data-search-tab='documents']")
    const authorBtn  = document.querySelector("#search-offcanvas-tabs [data-search-tab='authors']")

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

const initialType = syncSidebarSearchTypeFromPane()
setActivePanelInputs(initialType, SIDEBAR_DOC_PANEL, SIDEBAR_AUTH_PANEL, SIDEBAR_TYPE_INPUT)

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
