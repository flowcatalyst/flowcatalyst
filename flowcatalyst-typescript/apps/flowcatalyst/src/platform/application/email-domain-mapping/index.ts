/**
 * Email Domain Mapping Application Layer
 */

export type { CreateEmailDomainMappingCommand } from "./create-mapping/command.js";
export {
	type CreateEmailDomainMappingUseCaseDeps,
	createCreateEmailDomainMappingUseCase,
} from "./create-mapping/use-case.js";

export type { UpdateEmailDomainMappingCommand } from "./update-mapping/command.js";
export {
	type UpdateEmailDomainMappingUseCaseDeps,
	createUpdateEmailDomainMappingUseCase,
} from "./update-mapping/use-case.js";

export type { DeleteEmailDomainMappingCommand } from "./delete-mapping/command.js";
export {
	type DeleteEmailDomainMappingUseCaseDeps,
	createDeleteEmailDomainMappingUseCase,
} from "./delete-mapping/use-case.js";
