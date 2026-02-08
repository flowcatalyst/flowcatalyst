<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkCreateRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\CreateRoleRequest $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/api/sdk/roles';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\CreateRoleRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleConflictException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\RoleDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RoleDto', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateRoleBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateRoleNotFoundException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateRoleConflictException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateRoleUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateRoleForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}