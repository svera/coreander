"use strict"

import {
    applyHiddenDatesToVisible,
    bindOffcanvasFilterSync,
    enableFilterInputsOnPageShow,
    initSearchFilters,
    syncSidebarFormToOffcanvas,
} from './search-filter-utils.js'

function authorSyncOffcanvas() {
    syncSidebarFormToOffcanvas({
        searchFieldName: 'search',
        offcanvasContainerId: 'author-search-filters',
    })
}

function initAuthorSearchFilters(searchFilters) {
    initSearchFilters(searchFilters, { syncOffcanvas: authorSyncOffcanvas })
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
