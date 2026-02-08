<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkUpdateRole extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $roleName;
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\UpdateRoleRequest $requestBody
     */
    public function __construct(string $roleName, ?\FlowCatalyst\Generated\Model\UpdateRoleRequest $requestBody = null)
    {
        $this->roleName = $roleName;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{roleName}'], [$this->roleName], '/api/sdk/roles/{roleName}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\UpdateRoleRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\RoleDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\RoleDto', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdateRoleNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdateRoleUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdateRoleForbiddenException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdateRoleBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}