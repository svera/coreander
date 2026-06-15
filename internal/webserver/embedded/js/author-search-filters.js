"use strict"

import {
    applyHiddenDatesToVisible,
    bindOffcanvasFilterSync,
    enableFilterInputsOnPageShow,
    initDateControls,
    initFilterFormBehavior,
    syncSidebarFormToOffcanvas,
} from './search-filter-utils.js'

function authorSyncOffcanvas() {
    syncSidebarFormToOffcanvas({
        searchFieldName: 'name',
        offcanvasContainerId: 'author-search-filters',
    })
}

function initAuthorSearchFilters(searchFilters) {
    if (!searchFilters) return
    const searchFiltersForm = searchFilters.closest('form')
    if (!searchFiltersForm) return

    const composeDateControls = initDateControls(searchFilters, searchFiltersForm)
    searchFiltersForm._coreanderComposeDates = searchFiltersForm._coreanderComposeDates || []
    searchFiltersForm._coreanderComposeDates.push(composeDateControls)
    if (searchFiltersForm.dataset.coreanderFilterBehavior === 'true') {
        return
    }
    initFilterFormBehavior({
        searchFilters,
        searchFiltersForm,
        composeDateControls,
        listPath: '/search',
        syncOffcanvas: authorSyncOffcanvas,
    })
    searchFiltersForm.dataset.coreanderFilterBehavior = 'true'
}

enableFilterInputsOnPageShow(['author-search-filters', 'author-search-filters-sidebar'])

initAuthorSearchFilters(document.getElementById('author-search-filters'))
initAuthorSearchFilters(document.getElementById('author-search-filters-sidebar'))

bindOffcanvasFilterSync({
    sidebarFormId: 'search-filters-form',
    offcanvasContainerId: 'author-search-filters',
    offcanvasElementId: 'search-filters-offcanvas',
    syncOffcanvas: authorSyncOffcanvas,
})
