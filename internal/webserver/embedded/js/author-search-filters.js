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

enableFilterInputsOnPageShow(['author-search-filters', 'author-search-filters-sidebar'])

initSearchFilters(document.getElementById('author-search-filters'), { syncOffcanvas: authorSyncOffcanvas })
initSearchFilters(document.getElementById('author-search-filters-sidebar'), { syncOffcanvas: authorSyncOffcanvas })

bindOffcanvasFilterSync({
    sidebarFormId: 'search-filters-form',
    offcanvasContainerId: 'author-search-filters',
    offcanvasElementId: 'search-filters-offcanvas',
    syncOffcanvas: authorSyncOffcanvas,
})
