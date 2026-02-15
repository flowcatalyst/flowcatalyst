<?php

namespace FlowCatalyst\Generated\Endpoint;

class ProvisionApplicationServiceAccount extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * Creates a service account and OAuth client for an application that doesn't have one. The client secret is only returned once and cannot be retrieved later.
     * @param string $id
     */
    public function __construct(string $id)
    {
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/applications/{id}/provision-service-account');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\ProvisionServiceAccountResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ProvisionServiceAccountResponse', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\NotFoundResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\UnauthorizedResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ForbiddenResponse', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}