<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiAdminRoleByName extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $name;
    /**
     * @param string $name
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody $requestBody
     */
    public function __construct(string $name, ?\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody $requestBody = null)
    {
        $this->name = $name;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{name}'], [$this->name], '/api/admin/roles/{name}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody) {
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
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse200', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse404', 'json'), $response);
        }
        if (is_null($contentType) === false && (409 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameConflictException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse409', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}