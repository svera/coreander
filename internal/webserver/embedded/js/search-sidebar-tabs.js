"use strict"

import { syncSidebarSearchTypeFromPane, tabTypeFromEvent, setActivePanelInputs } from './search-filter-utils.js'

const DOC_PANEL  = 'search-sidebar-documents-panel'
const AUTH_PANEL = 'search-sidebar-authors-panel'
const TYPE_INPUT = 'search-type'

function initSearchSidebarTabs() {
    const tabs = document.getElementById("search-sidebar-tabs")
    const form = document.getElementById("search-filters-form")
    if (!tabs || !form) return

    let activeType = syncSidebarSearchTypeFromPane()
    setActivePanelInputs(activeType, DOC_PANEL, AUTH_PANEL, TYPE_INPUT)

    tabs.addEventListener("show.bs.tab", (event) => {
        const nextType = tabTypeFromEvent(event, '[data-search-tab]', 'searchTab')
        if (!nextType || nextType === activeType) return
        activeType = nextType
        setActivePanelInputs(nextType, DOC_PANEL, AUTH_PANEL, TYPE_INPUT)
    })

    tabs.addEventListener("shown.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event, '[data-search-tab]', 'searchTab')
        if (!tabName) return
        activeType = tabName
        setActivePanelInputs(tabName, DOC_PANEL, AUTH_PANEL, TYPE_INPUT)
        if (form._coreanderApplyFilters) {
            form._coreanderApplyFilters()
        } else if (window.htmx) {
            window.htmx.trigger(document.body, "update")
        }
    })
}

initSearchSidebarTabs()
