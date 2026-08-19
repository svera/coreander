"use strict"

import { initClearAllFilters, initClearRangeControls, initDateControls, syncSearchTypeFromPane, tabTypeFromEvent, setActivePanelInputs } from './search-filter-utils.js'

const STORAGE_KEY = "coreander-home-search-tab"
const DOC_PANEL  = 'home-search-documents-panel'
const AUTH_PANEL = 'home-search-authors-panel'
const TYPE_INPUT = 'home-search-type'

function getStoredTab() {
    try {
        const stored = sessionStorage.getItem(STORAGE_KEY)
        if (stored === "documents" || stored === "authors") {
            return stored
        }
    } catch (_) {
        // ignore storage errors
    }
    return "documents"
}

function isAdvancedSearchOpen(collapse) {
    if (collapse?.classList.contains("show")) return true
    const toggle = document.querySelector('[href="#home-advanced-search-collapse"], [data-bs-target="#home-advanced-search-collapse"]')
    return toggle?.getAttribute("aria-expanded") === "true"
}

function resolveSearchType(collapse) {
    if (isAdvancedSearchOpen(collapse)) {
        return syncSearchTypeFromPane("home-search-type", "home-search-authors-panel")
    }
    const typeInput = document.getElementById(TYPE_INPUT)
    if (typeInput) typeInput.value = "documents"
    return "documents"
}

function hasFilterParams(params) {
    for (const key of params.keys()) {
        if (key !== "type") return true
    }
    return false
}

function collectPanelParams(panel, composeDateControls, type) {
    if (!panel) return new URLSearchParams()

    composeDateControls()
    const params = new URLSearchParams()
    params.set("type", type)
    panel.querySelectorAll("input, select, textarea").forEach((el) => {
        if (!el.name || el.disabled) return
        if ((el.type === "checkbox" || el.type === "radio") && !el.checked) return
        const value = String(el.value ?? "").trim()
        if (value === "" || value === "0") return
        params.append(el.name, value)
    })
    return params
}

function restoreStoredTab() {
    try {
        const stored = sessionStorage.getItem(STORAGE_KEY)
        if (stored !== "authors") return
        const authorsTab = document.getElementById("home-search-tab-authors")
        if (authorsTab && typeof bootstrap !== "undefined") {
            bootstrap.Tab.getOrCreateInstance(authorsTab).show()
        }
    } catch (_) {
        // ignore storage errors
    }
}

function initHomeSearchTabs() {
    const tabs = document.getElementById("home-search-tabs")
    if (!tabs) return

    let activeType = getStoredTab()
    setActivePanelInputs(activeType, DOC_PANEL, AUTH_PANEL, TYPE_INPUT)

    tabs.addEventListener("show.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event, '[data-home-search-tab]', 'homeSearchTab')
        if (!tabName) return
        activeType = tabName
        setActivePanelInputs(tabName, DOC_PANEL, AUTH_PANEL, TYPE_INPUT)
    })

    tabs.addEventListener("shown.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event, '[data-home-search-tab]', 'homeSearchTab')
        if (!tabName) return
        try {
            sessionStorage.setItem(STORAGE_KEY, tabName)
        } catch (_) {
            // ignore storage errors
        }
    })

    restoreStoredTab()
}

function initHomeSearch() {
    const form = document.getElementById("home-search-form")
    if (!form) return

    const collapse = document.getElementById("home-advanced-search-collapse")
    const docPanel = document.getElementById("home-search-documents-panel")
    const authorPanel = document.getElementById("home-search-authors-panel")
    const searchbox = form.querySelector("#searchbox")

    const composeDocumentDates = docPanel ? initDateControls(docPanel, form) : () => {}
    const composeAuthorDates = authorPanel ? initDateControls(authorPanel, form) : () => {}
    form._coreanderComposeDates = [composeDocumentDates, composeAuthorDates]

    const docFilters = docPanel?.querySelector("#document-search-filters")
    const authorFilters = authorPanel?.querySelector("#author-search-filters")
    if (docFilters) {
        initClearRangeControls(docFilters)
        initClearAllFilters(docFilters, form)
    }
    if (authorFilters) {
        initClearRangeControls(authorFilters)
        initClearAllFilters(authorFilters, form)
    }

    form.addEventListener("submit", (event) => {
        event.preventDefault()
        const query = searchbox?.value.trim() ?? ""
        const searchType = resolveSearchType(collapse)

        if (!isAdvancedSearchOpen(collapse)) {
            if (!query) return
            const params = new URLSearchParams({ type: searchType, search: query })
            window.location.href = "/search?" + params.toString()
            return
        }

        const composeDates = searchType === "authors" ? composeAuthorDates : composeDocumentDates
        const panel = searchType === "authors" ? authorPanel : docPanel

        const params = collectPanelParams(panel, composeDates, searchType)
        if (query) params.set("search", query)
        if (!query && !params.has("name") && !hasFilterParams(params)) return
        window.location.href = "/search?" + params.toString()
    })

    initHomeSearchTabs()

    if (docFilters) {
        import("./document-search-filters.js")
            .then(({ initSubjectsFilters }) => initSubjectsFilters(docFilters, "", null))
            .catch(() => {})
    }
}

initHomeSearch()
