"use strict"

import { enableFilterInputsOnPageShow, initSearchFilters } from './search-filter-utils.js'
import { initSubjectsFilters } from './document-search-filters.js'

enableFilterInputsOnPageShow(['highlight-search-filters'])

initSearchFilters(document.getElementById('highlight-search-filters'), {
    onInit: (schedule) => initSubjectsFilters(document.getElementById('highlight-search-filters'), 'highlight-', schedule),
})
