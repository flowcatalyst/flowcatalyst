import { ResultAsync, errAsync, okAsync } from 'neverthrow';
import type { Logger } from '@flowcatalyst/logging';
import {
	ElasticLoadBalancingV2Client,
	RegisterTargetsCommand,
	DeregisterTargetsCommand,
	DescribeTargetHealthCommand,
} from '@aws-sdk/client-elastic-load-balancing-v2';
import type { TrafficManagementStrategy } from './strategy.js';
import type { TrafficError } from './errors.js';
import { TrafficErrors } from './errors.js';

/**
 * AWS ALB strategy configuration
 */
export interface AwsAlbStrategyConfig {
	/** AWS region */
	region: string;
	/** Target group ARN */
	targetGroupArn: string;
	/** Target ID (typically the EC2 instance ID or IP address) */
	targetId: string;
	/** Target port (defaults to 8080) */
	targetPort?: number | undefined;
	/** Deregistration timeout in seconds (defaults to 300) */
	deregistrationDelaySeconds?: number | undefined;
}

/**
 * Internal resolved config with all defaults applied
 */
interface ResolvedAwsAlbConfig {
	region: string;
	targetGroupArn: string;
	targetId: string;
	targetPort: number;
	deregistrationDelaySeconds: number;
}

/**
 * AWS ALB target group deregistration strategy
 *
 * Handles registration/deregistration from an ALB target group
 * for standby mode support.
 *
 * Matches Java AwsAlbDeregistrationStrategy behavior.
 */
export class AwsAlbStrategy implements TrafficManagementStrategy {
	private readonly config: ResolvedAwsAlbConfig;
	private readonly client: ElasticLoadBalancingV2Client;
	private readonly logger: Logger;
	private registered = false;

	constructor(config: AwsAlbStrategyConfig, logger: Logger) {
		this.config = {
			...config,
			targetPort: config.targetPort ?? 8080,
			deregistrationDelaySeconds: config.deregistrationDelaySeconds ?? 300,
		};
		this.logger = logger.child({
			component: 'AwsAlbStrategy',
			targetGroupArn: config.targetGroupArn,
			targetId: config.targetId,
		});
		this.client = new ElasticLoadBalancingV2Client({
			region: config.region,
		});

		this.logger.info(
			{
				targetPort: this.config.targetPort,
				deregistrationDelaySeconds: this.config.deregistrationDelaySeconds,
			},
			'AWS ALB strategy initialized',
		);
	}

	getName(): string {
		return 'AWS_ALB_DEREGISTRATION';
	}

	registerAsActive(): ResultAsync<void, TrafficError> {
		this.logger.info('Registering target with ALB target group');

		return ResultAsync.fromPromise(
			this.client.send(
				new RegisterTargetsCommand({
					TargetGroupArn: this.config.targetGroupArn,
					Targets: [
						{
							Id: this.config.targetId,
							Port: this.config.targetPort,
						},
					],
				}),
			),
			(error) =>
				TrafficErrors.registrationFailed(
					this.getName(),
					error instanceof Error ? error : new Error(String(error)),
				),
		).map(() => {
			this.registered = true;
			this.logger.info('Target registered with ALB target group');
		});
	}

	deregisterFromActive(): ResultAsync<void, TrafficError> {
		this.logger.info('Deregistering target from ALB target group');

		return ResultAsync.fromPromise(
			this.client.send(
				new DeregisterTargetsCommand({
					TargetGroupArn: this.config.targetGroupArn,
					Targets: [
						{
							Id: this.config.targetId,
							Port: this.config.targetPort,
						},
					],
				}),
			),
			(error) =>
				TrafficErrors.deregistrationFailed(
					this.getName(),
					error instanceof Error ? error : new Error(String(error)),
				),
		).andThen(() => this.waitForDeregistration());
	}

	isRegistered(): boolean {
		return this.registered;
	}

	/**
	 * Wait for target to be fully deregistered (draining complete)
	 */
	private waitForDeregistration(): ResultAsync<void, TrafficError> {
		const maxWaitMs = this.config.deregistrationDelaySeconds * 1000;
		const pollIntervalMs = 5000;
		const startTime = Date.now();

		const checkHealth = (): ResultAsync<void, TrafficError> => {
			const elapsed = Date.now() - startTime;
			if (elapsed > maxWaitMs) {
				this.registered = false;
				this.logger.warn(
					{ elapsedMs: elapsed, maxWaitMs },
					'Deregistration wait timed out, proceeding anyway',
				);
				return okAsync(undefined);
			}

			return ResultAsync.fromPromise(
				this.client.send(
					new DescribeTargetHealthCommand({
						TargetGroupArn: this.config.targetGroupArn,
						Targets: [
							{
								Id: this.config.targetId,
								Port: this.config.targetPort,
							},
						],
					}),
				),
				(error) =>
					TrafficErrors.deregistrationFailed(
						this.getName(),
						error instanceof Error ? error : new Error(String(error)),
					),
			).andThen((response) => {
				const health = response.TargetHealthDescriptions?.[0];
				const state = health?.TargetHealth?.State;

				this.logger.debug(
					{ state, elapsedMs: elapsed },
					'Checking target health during deregistration',
				);

				// Target is fully deregistered when not found or unused
				if (!health || state === 'unused' || state === 'draining') {
					if (state !== 'draining') {
						this.registered = false;
						this.logger.info('Target deregistered from ALB target group');
						return okAsync(undefined);
					}
				}

				// Still draining, wait and check again
				return ResultAsync.fromPromise(
					new Promise((resolve) => setTimeout(resolve, pollIntervalMs)),
					() =>
						TrafficErrors.deregistrationFailed(
							this.getName(),
							new Error('Wait interrupted'),
						),
				).andThen(() => checkHealth());
			});
		};

		return checkHealth();
	}
}

/**
 * Create AWS ALB strategy from environment configuration
 */
export function createAwsAlbStrategy(
	config: AwsAlbStrategyConfig,
	logger: Logger,
): AwsAlbStrategy {
	return new AwsAlbStrategy(config, logger);
}
