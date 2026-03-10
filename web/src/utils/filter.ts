export const combineFilters = (...filters: Array<string | undefined>) => {
  const conditions = filters.filter((filter): filter is string => Boolean(filter && filter.trim()));
  if (conditions.length === 0) {
    return undefined;
  }
  return conditions.map((condition) => `(${condition})`).join(" && ");
};
