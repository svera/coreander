"use strict"

import { syncSidebarSearchTypeFromPane } from './search-filter-utils.js'

function setSearchType(type) {
    const typeInput = document.getElementById("search-type")
    if (typeInput) typeInput.value = type
}

function tabTypeFromEvent(event) {
    const tabBtn = event.target?.closest?.("[data-search-tab]") ?? event.target
    const tabName = tabBtn?.dataset?.searchTab
    return tabName === "authors" || tabName === "documents" ? tabName : null
}

function setActivePanelInputs(activeType) {
    const docPanel = document.getElementById("search-sidebar-documents-panel")
    const authorPanel = document.getElementById("search-sidebar-authors-panel")

    docPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        if (el.id === "search-type") return
        el.disabled = activeType !== "documents"
    })
    authorPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        el.disabled = activeType !== "authors"
    })
}

function initSearchSidebarTabs() {
    const tabs = document.getElementById("search-sidebar-tabs")
    const form = document.getElementById("search-filters-form")
    if (!tabs || !form) return

    let activeType = syncSidebarSearchTypeFromPane()
    setActivePanelInputs(activeType)

    tabs.addEventListener("show.bs.tab", (event) => {
        const nextType = tabTypeFromEvent(event)
        if (!nextType || nextType === activeType) return
        activeType = nextType
        setSearchType(nextType)
        setActivePanelInputs(nextType)
    })

    tabs.addEventListener("shown.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event)
        if (!tabName) return
        activeType = tabName
        setSearchType(tabName)
        setActivePanelInputs(tabName)
        if (window.htmx) {
            window.htmx.trigger(document.body, "update")
        }
    })
}

initSearchSidebarTabs()
